package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shipit-ai-demo-org/inventory-service/internal/store"
)

type StockHandler struct {
	store *store.Store
}

func NewStockHandler(s *store.Store) *StockHandler {
	return &StockHandler{store: s}
}

// GetStock handles GET /v1/stock/{sku}.
func (h *StockHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	sku := r.PathValue("sku")
	rec, err := h.store.GetStock(sku)
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

	rec := h.store.UpsertStock(sku, body.Warehouse, body.OnHand)
	writeJSON(w, http.StatusOK, rec)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
