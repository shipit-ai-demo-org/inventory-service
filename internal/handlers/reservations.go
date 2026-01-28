package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/shipit-ai-demo-org/inventory-service/internal/store"
)

const defaultReservationTTL = 30 * time.Minute

type ReservationHandler struct {
	store *store.Store
}

func NewReservationHandler(s *store.Store) *ReservationHandler {
	return &ReservationHandler{store: s}
}

// Create handles POST /v1/reservations. Called by the order.created consumer
// and directly by ops tooling for manual holds.
func (h *ReservationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID    string `json:"orderId"`
		SKU        string `json:"sku"`
		Quantity   int    `json:"quantity"`
		TTLSeconds int    `json:"ttlSeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if body.Quantity <= 0 || body.SKU == "" || body.OrderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_reservation"})
		return
	}

	ttl := defaultReservationTTL
	if body.TTLSeconds > 0 {
		ttl = time.Duration(body.TTLSeconds) * time.Second
	}

	res, err := h.store.Reserve(body.OrderID, body.SKU, body.Quantity, ttl)
	if err != nil {
		if errors.Is(err, store.ErrUnknownSKU) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_sku"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// Release handles DELETE /v1/reservations/{id}.
func (h *ReservationHandler) Release(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.Release(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_reservation"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
