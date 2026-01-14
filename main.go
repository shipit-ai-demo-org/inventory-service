package main

import (
	"log"
	"net/http"
	"os"

	"github.com/shipit-ai-demo-org/inventory-service/internal/handlers"
	"github.com/shipit-ai-demo-org/inventory-service/internal/store"
)

func main() {
	st := store.New()
	stockHandler := handlers.NewStockHandler(st)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"inventory-service"}`))
	})

	mux.HandleFunc("GET /v1/stock/{sku}", stockHandler.GetStock)
	mux.HandleFunc("PUT /v1/stock/{sku}", stockHandler.UpsertStock)

	addr := ":" + envOr("PORT", "8080")
	log.Printf("inventory-service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
