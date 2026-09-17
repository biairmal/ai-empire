// Command controlplane is the AI Dev OS control plane (HTTP API + scheduler).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"aiempire/internal/controlplane"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stale, err := time.ParseDuration(env("EMPIRE_STALE_AFTER", "60s"))
	if err != nil {
		log.Fatalf("EMPIRE_STALE_AFTER: %v", err)
	}
	pool, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://empire:empire@localhost:5433/empire?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database: %v (is `make up` running?)", err)
	}

	srv, err := controlplane.New(pool, controlplane.Config{
		OwnerToken:   os.Getenv("EMPIRE_OWNER_TOKEN"),
		WorkerToken:  os.Getenv("EMPIRE_WORKER_TOKEN"),
		KnowledgeDir: env("EMPIRE_KNOWLEDGE_DIR", "knowledge"),
		WorkflowsDir: env("EMPIRE_WORKFLOWS_DIR", "workflows"),
		StaleAfter:   stale,
	})
	if err != nil {
		log.Fatalf("config: %v (set EMPIRE_OWNER_TOKEN and EMPIRE_WORKER_TOKEN, see .env.example)", err)
	}
	if err := srv.Reindex(ctx); err != nil {
		log.Printf("knowledge reindex: %v", err)
	}
	go srv.RunReaper(ctx)

	hs := &http.Server{
		Addr:              env("CP_ADDR", ":8787"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	log.Printf("control plane listening on %s", hs.Addr)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
