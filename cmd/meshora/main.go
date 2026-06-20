package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/djkazic/meshora/internal/geo"
	"github.com/djkazic/meshora/internal/ingest"
	"github.com/djkazic/meshora/internal/server"
	"github.com/djkazic/meshora/internal/store"
	"github.com/djkazic/meshora/web"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "HTTP listen address")
		dbPath  = flag.String("db", "meshora.db", "SQLite database path")
		webDir  = flag.String("web", "", "serve frontend from this directory instead of the embedded build")
		source  = flag.String("source", "feed", "ingestion source: feed | broker")
		feedURL = flag.String("feed-url", "wss://analyzer.bostonme.sh/", "analyzer WebSocket feed URL (source=feed)")
		boot    = flag.Bool("bootstrap", true, "seed nodes once from the analyzer REST API (source=feed)")
		broker  = flag.String("broker", "", "MQTT broker URL, e.g. wss://host:443/mqtt (source=broker)")
		user    = flag.String("user", "", "MQTT username (source=broker)")
		pass    = flag.String("pass", "", "MQTT password or JWT (source=broker)")
		topic   = flag.String("topic", "meshcore/#", "MQTT subscription topic (source=broker)")
	)
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	hub := server.NewHub()
	proc, err := ingest.NewProcessor(st, hub)
	if err != nil {
		log.Fatalf("processor: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch *source {
	case "feed":
		if *boot {
			n, err := ingest.Bootstrap(ctx, st, *feedURL)
			if err != nil {
				log.Printf("bootstrap: partial/failed: %v", err)
			}
			_ = proc.ReloadPositions()
			log.Printf("bootstrapped %d node/observer identities", n)
		}
		go ingest.NewFeedSource(*feedURL, proc).Run(ctx)
	case "broker":
		if *broker == "" {
			log.Fatal("source=broker requires -broker")
		}
		go func() {
			if err := ingest.NewBrokerSource(*broker, *user, *pass, *topic, proc).Run(ctx); err != nil {
				log.Printf("broker source stopped: %v", err)
			}
		}()
	default:
		log.Fatalf("unknown source %q (want feed or broker)", *source)
	}

	var staticFS fs.FS
	if *webDir != "" {
		staticFS = os.DirFS(*webDir)
		log.Printf("serving frontend from disk: %s", *webDir)
	} else {
		staticFS = web.DistFS()
	}

	srv := &http.Server{
		Addr:         *addr,
		Handler:      server.New(st, hub, geo.GreaterBoston, staticFS).Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
	}

	go func() {
		log.Printf("meshora listening on %s (source=%s)", *addr, *source)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
