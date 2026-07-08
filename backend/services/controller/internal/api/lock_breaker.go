package api

import (
	"sync"
	"time"
)

// lockCircuitBreaker is a per-tenant lock-rate circuit breaker.
// When the number of successful LOCK deliveries in the sliding window exceeds
// the threshold, the breaker trips (latch-open) and suppresses further LOCK
// commands until reset via config update or batch whitelist import.
// UNLOCK commands are never affected.
type lockCircuitBreaker struct {
	mu        sync.RWMutex
	windows   map[string]*lockRateWindow
	threshold int
	windowSec int
	enabled   bool
}

type lockRateWindow struct {
	tripped    bool
	timestamps []time.Time
}

func newLockCircuitBreaker() *lockCircuitBreaker {
	return &lockCircuitBreaker{
		windows:   make(map[string]*lockRateWindow),
		threshold: 500,
		windowSec: 60,
		enabled:   true,
	}
}

// recordLock appends a successful LOCK delivery timestamp and prunes entries
// outside the window.
func (b *lockCircuitBreaker) recordLock(tenantSlug string) {
	if !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	w, ok := b.windows[tenantSlug]
	if !ok {
		w = &lockRateWindow{}
		b.windows[tenantSlug] = w
	}

	now := time.Now()
	cutoff := now.Add(-time.Duration(b.windowSec) * time.Second)

	// Prune old entries
	pruned := w.timestamps[:0]
	for _, t := range w.timestamps {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	w.timestamps = append(pruned, now)

	// Check threshold
	if len(w.timestamps) >= b.threshold {
		w.tripped = true
	}
}

// isTripped returns true if the breaker has been tripped for this tenant.
func (b *lockCircuitBreaker) isTripped(tenantSlug string) bool {
	if !b.enabled {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	w, ok := b.windows[tenantSlug]
	if !ok {
		return false
	}
	return w.tripped
}

// reset clears the tripped state for a tenant (called on config update or
// manual reset via API).
func (b *lockCircuitBreaker) reset(tenantSlug string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.windows, tenantSlug)
}

// setThreshold updates the threshold and window. Must be called before
// StartLockEngine or when config changes.
func (b *lockCircuitBreaker) setThreshold(threshold, windowSec int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if threshold > 0 {
		b.threshold = threshold
	}
	if windowSec > 0 {
		b.windowSec = windowSec
	}
}

func (b *lockCircuitBreaker) setEnabled(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = enabled
}
