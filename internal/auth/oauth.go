package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// oauthProvider describes an OAuth2 SSO provider. Providers are enabled only
// when their client ID + secret are present in the environment.
type oauthProvider struct {
	name                  string
	clientID, secret      string
	authURL, tokenURL     string
	userURL               string
	scope                 string
}

// baseURL is where this control plane is reachable (for OAuth redirect URIs).
func baseURL() string {
	return strings.TrimRight(os.Getenv("LOOKOUT_BASE_URL"), "/")
}

// loadOAuthProviders reads provider credentials from the environment:
//
//	LOOKOUT_OAUTH_GOOGLE_CLIENT_ID / _SECRET
//	LOOKOUT_OAUTH_GITHUB_CLIENT_ID / _SECRET
//	LOOKOUT_BASE_URL=https://monitor.example.com
func loadOAuthProviders() map[string]*oauthProvider {
	out := map[string]*oauthProvider{}
	if id := os.Getenv("LOOKOUT_OAUTH_GOOGLE_CLIENT_ID"); id != "" {
		out["google"] = &oauthProvider{
			name: "google", clientID: id, secret: os.Getenv("LOOKOUT_OAUTH_GOOGLE_CLIENT_SECRET"),
			authURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			tokenURL: "https://oauth2.googleapis.com/token",
			userURL:  "https://openidconnect.googleapis.com/v1/userinfo",
			scope:    "openid email profile",
		}
	}
	if id := os.Getenv("LOOKOUT_OAUTH_GITHUB_CLIENT_ID"); id != "" {
		out["github"] = &oauthProvider{
			name: "github", clientID: id, secret: os.Getenv("LOOKOUT_OAUTH_GITHUB_CLIENT_SECRET"),
			authURL:  "https://github.com/login/oauth/authorize",
			tokenURL: "https://github.com/login/oauth/access_token",
			userURL:  "https://api.github.com/user",
			scope:    "read:user user:email",
		}
	}
	return out
}

func (p *oauthProvider) redirectURI() string {
	return baseURL() + "/auth/" + p.name + "/callback"
}

// authCodeURL builds the provider's consent URL.
func (p *oauthProvider) authCodeURL(state string) string {
	v := url.Values{}
	v.Set("client_id", p.clientID)
	v.Set("redirect_uri", p.redirectURI())
	v.Set("response_type", "code")
	v.Set("scope", p.scope)
	v.Set("state", state)
	return p.authURL + "?" + v.Encode()
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// exchange swaps an auth code for an access token.
func (p *oauthProvider) exchange(code string) (string, error) {
	v := url.Values{}
	v.Set("client_id", p.clientID)
	v.Set("client_secret", p.secret)
	v.Set("code", code)
	v.Set("redirect_uri", p.redirectURI())
	v.Set("grant_type", "authorization_code")
	req, _ := http.NewRequest(http.MethodPost, p.tokenURL, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("token exchange failed")
	}
	return t.AccessToken, nil
}

// email fetches the verified email for the authenticated user.
func (p *oauthProvider) email(token string) (string, string, error) {
	req, _ := http.NewRequest(http.MethodGet, p.userURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var u struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Login         string `json:"login"`
	}
	_ = json.Unmarshal(body, &u)
	name := u.Name
	if name == "" {
		name = u.Login
	}
	// OIDC providers (e.g. Google) carry an email_verified claim — never trust an
	// unverified address, since it lets an attacker bind to someone else's email.
	// GitHub's /user response has no such field; its verified address is fetched
	// separately below via githubPrimaryEmail.
	if u.Email != "" && p.name != "github" {
		if !u.EmailVerified {
			return "", "", fmt.Errorf("provider returned an unverified email")
		}
		return strings.ToLower(u.Email), name, nil
	}
	// GitHub may not return a public email; fetch the primary verified one.
	if p.name == "github" {
		if e := githubPrimaryEmail(token); e != "" {
			return e, name, nil
		}
	}
	return "", "", fmt.Errorf("provider did not return a verified email")
}

func githubPrimaryEmail(token string) string {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	_ = json.Unmarshal(body, &emails)
	for _, e := range emails {
		if e.Primary && e.Verified {
			return strings.ToLower(e.Email)
		}
	}
	return ""
}
