package store

import (
	"context"
	"log"
	"time"
)

// DefaultSweepInterval is how often the background sweeper releases expired
// reservations when no interval is configured.
const DefaultSweepInterval = time.Minute

// SweepExpired releases all reservations past their TTL and reports how many
// were released. It is the exported counterpart of the inline sweep performed
// on Reserve, for callers that want expiry to happen even on idle SKUs.
func (s *Store) SweepExpired(ctx context.Context) (int, error) {
	if err := checkCtx(ctx); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := len(s.reservations)
	s.sweepExpiredLocked(time.Now().UTC())
	return before - len(s.reservations), nil
}

// Sweeper periodically releases expired reservations in the background.
//
// Until now expired holds were only reclaimed lazily, inside Reserve, so a
// SKU that received no new reservation traffic could sit with stale Reserved
// counts indefinitely — skewing ReplenishmentCandidates and the stock levels
// reported to ops dashboards (follow-up to INV-902).
type Sweeper struct {
	store    *Store
	interval time.Duration
}

// NewSweeper returns a sweeper for s. A non-positive interval falls back to
// DefaultSweepInterval.
func NewSweeper(s *Store, interval time.Duration) *Sweeper {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	return &Sweeper{store: s, interval: interval}
}

// Run blocks, sweeping on every tick until ctx is cancelled. Intended to be
// started as a goroutine from main.
func (sw *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			released, err := sw.store.SweepExpired(ctx)
			if err != nil {
				return // ctx cancelled between tick and sweep
			}
			if released > 0 {
				log.Printf("store: sweeper released %d expired reservation(s)", released)
			}
		}
	}
}
