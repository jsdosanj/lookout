// Command lookout-server is the Lookout control plane: it ingests agent reports
// and serves the dashboard (behind login + RBAC).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/notify"
	"github.com/jsdosanj/lookout/internal/server"
	"github.com/jsdosanj/lookout/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "lookout-data.json", "path to the server data file")
	authData := flag.String("auth-data", "lookout-users.json", "path to the users/sessions file")
	flag.Parse()

	token := os.Getenv("LOOKOUT_TOKEN")                            // agent enrollment token
	secureCookies := os.Getenv("LOOKOUT_SECURE_COOKIES") == "true" // set true behind TLS

	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	users, err := auth.Open(*authData)
	if err != nil {
		log.Fatalf("open auth store: %v", err)
	}
	bootstrapOwner(users)

	a := auth.New(users, secureCookies, "Lookout")
	// Alert webhooks (Slack/Teams/generic) from LOOKOUT_ALERT_WEBHOOKS (comma-separated).
	var webhooks []string
	for _, u := range strings.Split(os.Getenv("LOOKOUT_ALERT_WEBHOOKS"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			webhooks = append(webhooks, u)
		}
	}
	n := notify.New(webhooks)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(st, token, a, n).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Lookout control plane on %s (data: %s)", *addr, *data)
	if token == "" {
		log.Print("WARNING: LOOKOUT_TOKEN not set — agent reports are unauthenticated (dev only)")
	}
	if !secureCookies {
		log.Print("NOTE: LOOKOUT_SECURE_COOKIES != 'true' — cookies not marked Secure (run behind TLS in production)")
	}
	log.Fatal(srv.ListenAndServe())
}

// bootstrapOwner creates the first owner account on a fresh install from
// LOOKOUT_ADMIN_EMAIL + LOOKOUT_ADMIN_PASSWORD.
func bootstrapOwner(users *auth.Store) {
	if users.Count() > 0 {
		return
	}
	email, pw := os.Getenv("LOOKOUT_ADMIN_EMAIL"), os.Getenv("LOOKOUT_ADMIN_PASSWORD")
	if email == "" || pw == "" {
		log.Print("NOTE: no users yet. Set LOOKOUT_ADMIN_EMAIL and LOOKOUT_ADMIN_PASSWORD to create the first owner.")
		return
	}
	if _, err := users.CreateUser(email, "Owner", auth.RoleOwner, pw); err != nil {
		log.Printf("could not create owner: %v", err)
		return
	}
	log.Printf("created owner account %s", email)
}
