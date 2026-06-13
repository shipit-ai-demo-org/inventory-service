package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrUnknownSKU         = errors.New("unknown sku")
	ErrUnknownReservation = errors.New("unknown reservation")
	ErrInsufficientStock  = errors.New("insufficient available stock")
)

// StockRecord tracks on-hand and reserved quantities for a single SKU at a
// warehouse. Available = OnHand - Reserved.
type StockRecord struct {
	SKU       string    `json:"sku"`
	Warehouse string    `json:"warehouse"`
	OnHand    int       `json:"onHand"`
	Reserved  int       `json:"reserved"`
	// ReorderPoint is the available-stock threshold below which the SKU is
	// flagged for replenishment. Zero means replenishment tracking is off.
	ReorderPoint int       `json:"reorderPoint"`
	UpdatedAt    time.Time `json:"updatedAt"`
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

// checkCtx aborts an operation whose caller has already given up (request
// timeout, client disconnect, consumer shutdown). All store operations take a
// context so deadlines propagate end-to-end today and slot straight into the
// planned Postgres-backed store, where these become real query timeouts.
func checkCtx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetStock(ctx context.Context, sku string) (StockRecord, error) {
	if err := checkCtx(ctx); err != nil {
		return StockRecord{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.stock[sku]
	if !ok {
		return StockRecord{}, ErrUnknownSKU
	}
	return *rec, nil
}

func (s *Store) UpsertStock(ctx context.Context, sku, warehouse string, onHand int) (StockRecord, error) {
	if err := checkCtx(ctx); err != nil {
		return StockRecord{}, err
	}
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
	return *rec, nil
}

func (s *Store) Reserve(ctx context.Context, orderID, sku string, qty int, ttl time.Duration) (Reservation, error) {
	if err := checkCtx(ctx); err != nil {
		return Reservation{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Expired holds were inflating Reserved and starving new orders
	// (INV-902): sweep them before computing availability.
	s.sweepExpiredLocked(time.Now().UTC())

	rec, ok := s.stock[sku]
	if !ok {
		return Reservation{}, ErrUnknownSKU
	}

	// Never allow reserved to exceed on-hand: two pickers racing for the
	// last unit must not both win (INV-841).
	if available := rec.OnHand - rec.Reserved; qty > available {
		return Reservation{}, ErrInsufficientStock
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

func (s *Store) Release(ctx context.Context, reservationID string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
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

// sweepExpiredLocked releases holds past their TTL. Caller must hold s.mu.
func (s *Store) sweepExpiredLocked(now time.Time) {
	for id, res := range s.reservations {
		if now.After(res.ExpiresAt) {
			if rec, ok := s.stock[res.SKU]; ok {
				rec.Reserved -= res.Quantity
				rec.UpdatedAt = now
			}
			delete(s.reservations, id)
		}
	}
}

// SetReorderPoint configures the replenishment threshold for a SKU.
func (s *Store) SetReorderPoint(ctx context.Context, sku string, point int) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.stock[sku]
	if !ok {
		return ErrUnknownSKU
	}
	rec.ReorderPoint = point
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// ReplenishmentCandidates returns SKUs whose available stock has fallen below
// their reorder point. Consumed by the nightly replenishment planner.
func (s *Store) ReplenishmentCandidates(ctx context.Context) ([]StockRecord, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []StockRecord
	for _, rec := range s.stock {
		if rec.ReorderPoint > 0 && rec.OnHand-rec.Reserved < rec.ReorderPoint {
			out = append(out, *rec)
		}
	}
	return out, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
