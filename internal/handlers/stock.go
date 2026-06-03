package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/shipit-ai-demo-org/inventory-service/internal/store"
)

// storeTimeout caps how long a request may wait on a store operation, so a
// contended store cannot pin handler goroutines past the client's patience.
const storeTimeout = 2 * time.Second

type StockHandler struct {
	store *store.Store
}

func NewStockHandler(s *store.Store) *StockHandler {
	return &StockHandler{store: s}
}

// GetStock handles GET /v1/stock/{sku}.
func (h *StockHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	rec, err := h.store.GetStock(ctx, sku)
	if err != nil {
		if errors.Is(err, store.ErrUnknownSKU) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_sku"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// UpsertStock handles PUT /v1/stock/{sku} — used by warehouse intake when
// putaway completes.
func (h *StockHandler) UpsertStock(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")

	var body struct {
		Warehouse string `json:"warehouse"`
		OnHand    int    `json:"onHand"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if body.OnHand < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "negative_on_hand"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	rec, err := h.store.UpsertStock(ctx, sku, body.Warehouse, body.OnHand)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_timeout"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
