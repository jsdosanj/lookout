// Command lookout-server is the Lookout control plane: it ingests agent reports
// and serves the dashboard.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jsdosanj/servmonitor/internal/server"
	"github.com/jsdosanj/servmonitor/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "lookout-data.json", "path to the data file")
	flag.Parse()

	// The enrollment token authenticates agent reports. Set it in production.
	token := os.Getenv("LOOKOUT_TOKEN")

	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(st, token).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Lookout control plane on %s (data: %s)", *addr, *data)
	if token == "" {
		log.Print("WARNING: LOOKOUT_TOKEN not set — agent reports are unauthenticated (dev only)")
	}
	log.Fatal(srv.ListenAndServe())
}
