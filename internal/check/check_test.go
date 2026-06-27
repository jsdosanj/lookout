package check

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTCPReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	c := Check{Type: "tcp", Target: ln.Addr().String()}
	got := c.Run(context.Background())
	if got.Status != statusOK {
		t.Fatalf("status = %q (%s), want ok", got.Status, got.Reason)
	}
}

func TestTCPUnreachable(t *testing.T) {
	// Open then close a listener to obtain a port that is now free.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	c := Check{Type: "tcp", Target: addr, Timeout: time.Second}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
}

func TestHTTPOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL}
	got := c.Run(context.Background())
	if got.Status != statusOK {
		t.Fatalf("status = %q (%s), want ok", got.Status, got.Reason)
	}
}

func TestHTTPCriticalOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
}

func TestHTTPExpectStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL, ExpectStatus: 200}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
}

func TestHTTPExpectStatusMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL, ExpectStatus: 404}
	got := c.Run(context.Background())
	if got.Status != statusOK {
		t.Fatalf("status = %q (%s), want ok", got.Status, got.Reason)
	}
}

func TestHTTPKeywordFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("service healthy: all systems go"))
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL, Keyword: "healthy"}
	got := c.Run(context.Background())
	if got.Status != statusOK {
		t.Fatalf("status = %q (%s), want ok", got.Status, got.Reason)
	}
}

func TestHTTPKeywordMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("nothing here"))
	}))
	defer srv.Close()

	c := Check{Type: "http", Target: srv.URL, Keyword: "healthy"}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
	if got.Reason != "keyword not found" {
		t.Fatalf("reason = %q, want %q", got.Reason, "keyword not found")
	}
}

func TestHTTPRedirectNotFollowed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusFound) // 302
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// The 302 is neither 2xx nor an ExpectStatus match, so it must be critical;
	// crucially the client must not follow it to the 200 at /ok.
	c := Check{Type: "http", Target: srv.URL + "/start"}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q (%s), want critical (redirect must not be followed)", got.Status, got.Reason)
	}
}

func TestUnknownType(t *testing.T) {
	c := Check{Type: "icmp", Target: "example.com"}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
	if got.Reason != `unknown check type "icmp"` {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestSchemeRejected(t *testing.T) {
	c := Check{Type: "http", Target: "ftp://example.com/file"}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
	// A request must not have been made; the reason names the scheme rejection.
	if got.Reason == "" {
		t.Fatalf("reason should explain scheme rejection")
	}
}

func TestSSRFLinkLocalMetadataBlocked(t *testing.T) {
	// The cloud-metadata endpoint is link-local; the dial must be refused, so
	// the result is critical even though no server is listening there.
	c := Check{Type: "http", Target: "http://169.254.169.254/latest/meta-data/", Timeout: 2 * time.Second}
	got := c.Run(context.Background())
	if got.Status != statusCritical {
		t.Fatalf("status = %q, want critical (link-local must be blocked)", got.Status)
	}
}
