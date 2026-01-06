package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrUnknownSKU         = errors.New("unknown sku")
	ErrUnknownReservation = errors.New("unknown reservation")
)

// StockRecord tracks on-hand and reserved quantities for a single SKU at a
// warehouse. Available = OnHand - Reserved.
type StockRecord struct {
	SKU       string    `json:"sku"`
	Warehouse string    `json:"warehouse"`
	OnHand    int       `json:"onHand"`
	Reserved  int       `json:"reserved"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Reservation is a soft hold against stock, created when orders-api emits
// order.created and released on shipment or expiry.
type Reservation struct {
	ID        string    `json:"id"`
	SKU       string    `json:"sku"`
	OrderID   string    `json:"orderId"`
	Quantity  int       `json:"quantity"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Store struct {
	mu           sync.RWMutex
	stock        map[string]*StockRecord
	reservations map[string]*Reservation
}

func New() *Store {
	return &Store{
		stock:        make(map[string]*StockRecord),
		reservations: make(map[string]*Reservation),
	}
}

func (s *Store) GetStock(sku string) (StockRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.stock[sku]
	if !ok {
		return StockRecord{}, ErrUnknownSKU
	}
	return *rec, nil
}

func (s *Store) UpsertStock(sku, warehouse string, onHand int) StockRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.stock[sku]
	if !ok {
		rec = &StockRecord{SKU: sku, Warehouse: warehouse}
		s.stock[sku] = rec
	}
	rec.OnHand = onHand
	rec.Warehouse = warehouse
	rec.UpdatedAt = time.Now().UTC()
	return *rec
}

func (s *Store) Reserve(orderID, sku string, qty int, ttl time.Duration) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.stock[sku]
	if !ok {
		return Reservation{}, ErrUnknownSKU
	}

	res := &Reservation{
		ID:        newID(),
		SKU:       sku,
		OrderID:   orderID,
		Quantity:  qty,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	rec.Reserved += qty
	rec.UpdatedAt = time.Now().UTC()
	s.reservations[res.ID] = res
	return *res, nil
}

func (s *Store) Release(reservationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, ok := s.reservations[reservationID]
	if !ok {
		return ErrUnknownReservation
	}
	if rec, ok := s.stock[res.SKU]; ok {
		rec.Reserved -= res.Quantity
		rec.UpdatedAt = time.Now().UTC()
	}
	delete(s.reservations, reservationID)
	return nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
