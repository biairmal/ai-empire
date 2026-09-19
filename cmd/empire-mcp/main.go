// Command empire-mcp is the MCP server Hermes uses to operate the control plane (stdio transport).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"aiempire/internal/client"
	"aiempire/internal/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log.SetOutput(os.Stderr) // stdout carries the protocol

	token := os.Getenv("EMPIRE_HERMES_TOKEN")
	if token == "" {
		log.Fatal("EMPIRE_HERMES_TOKEN is not set (see .env.example)")
	}
	cp := os.Getenv("EMPIRE_CP_URL")
	if cp == "" {
		cp = "http://localhost:8787"
	}
	s := &mcp.Server{C: client.New(cp, token)}
	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
