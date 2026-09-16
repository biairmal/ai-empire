// Command controlplane is the AI Dev OS control plane (HTTP API + scheduler).
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("CP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Printf("control plane listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
