// Command worker claims tasks from the control plane and runs them in isolated workspaces.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"

	"aiempire/internal/client"
	"aiempire/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	token := os.Getenv("EMPIRE_WORKER_TOKEN")
	if token == "" {
		log.Fatal("EMPIRE_WORKER_TOKEN is not set (see .env.example)")
	}
	host, _ := os.Hostname()

	var agent worker.Agent
	switch a := env("EMPIRE_AGENT", "claude"); a {
	case "claude":
		agent = worker.ClaudeCode{Model: os.Getenv("EMPIRE_AGENT_MODEL")}
	case "fake":
		agent = worker.Fake{}
	default:
		log.Fatalf("EMPIRE_AGENT: unknown agent %q (claude|fake)", a)
	}

	w, err := worker.New(client.New(env("EMPIRE_CP_URL", "http://localhost:8080"), token), worker.Config{
		Name:         env("EMPIRE_WORKER_NAME", host),
		Capabilities: strings.Split(env("EMPIRE_WORKER_CAPS", "go"), ","),
		Workspaces:   env("EMPIRE_WORKSPACES", "workspaces"),
		Agent:        agent,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
