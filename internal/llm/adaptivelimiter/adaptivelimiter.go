// Package adaptivelimiter implements a self-tuning concurrency limiter for
// calls to a single AI provider connector, shared across every review
// currently running against that connector. LiveReview can run many reviews
// concurrently (internal/jobqueue/queue_config.go's reviewWorkers), so a
// per-review concurrency cap alone can't protect a shared provider API key
// from being overwhelmed - this is the process-wide second tier.
//
// It uses an AIMD (additive-increase / multiplicative-decrease) schedule, the
// same algorithm family as TCP congestion control: on a run of successful
// calls it slowly raises how much concurrency it allows; the moment a call
// is classified as rate-limited (see internal/llm/errclass), it immediately
// halves the allowance. This means nobody has to guess the provider's real
// rate limit up front - the limiter discovers it by probing just past it and
// backing off, and settles near the real ceiling on its own.
package adaptivelimiter

import (
	"context"
	"sync"
	"time"
)

// Config bounds a Limiter. Floor and Ceiling are safety bounds, not targets -
// the limiter is free to settle anywhere between them.
type Config struct {
	// Floor is the minimum allowed concurrency; the limiter never drops
	// below this even under sustained rate-limiting. Defaults to 1.
	Floor int
	// Ceiling is a safety cap on concurrency. Defaults to Floor if unset or
	// smaller than Floor.
	Ceiling int
	// Initial is the starting capacity, clamped into [Floor, Ceiling].
	// Defaults to Floor.
	Initial int
	// IncreaseEvery is how many consecutive successful calls are required
	// before capacity is raised by 1. Defaults to 5.
	IncreaseEvery int
	// PollInterval is how often Acquire re-checks for a free slot while
	// waiting. Defaults to 20ms; tests may set this lower for speed.
	PollInterval time.Duration
}

// Limiter is a self-tuning concurrency limiter. Safe for concurrent use.
type Limiter struct {
	mu       sync.Mutex
	inFlight int
	capacity int
	floor    int
	ceiling  int

	successStreak int
	increaseEvery int
	pollInterval  time.Duration
}

// New creates a Limiter from cfg, applying defaults for any zero fields.
func New(cfg Config) *Limiter {
	floor := max(cfg.Floor, 1)
	ceiling := max(cfg.Ceiling, floor)
	initial := max(cfg.Initial, floor)
	initial = min(initial, ceiling)
	increaseEvery := cfg.IncreaseEvery
	if increaseEvery <= 0 {
		increaseEvery = 5
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 20 * time.Millisecond
	}

	return &Limiter{
		capacity:      initial,
		floor:         floor,
		ceiling:       ceiling,
		increaseEvery: increaseEvery,
		pollInterval:  pollInterval,
	}
}

// Acquire blocks until a slot is available or ctx is done. On success it
// returns a release function the caller must call exactly once when the
// gated call finishes, regardless of outcome (defer it immediately).
func (l *Limiter) Acquire(ctx context.Context) (release func(), err error) {
	for {
		l.mu.Lock()
		if l.inFlight < l.capacity {
			l.inFlight++
			l.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					l.mu.Lock()
					l.inFlight--
					l.mu.Unlock()
				})
			}, nil
		}
		poll := l.pollInterval
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return func() {}, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// ReportSuccess records a successful call. After IncreaseEvery consecutive
// successes, capacity is raised by 1 (capped at the configured ceiling).
func (l *Limiter) ReportSuccess() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successStreak++
	if l.successStreak >= l.increaseEvery && l.capacity < l.ceiling {
		l.capacity++
		l.successStreak = 0
	}
}

// ReportRateLimited records a call the caller classified as rate-limited
// (see internal/llm/errclass.Classify). Capacity is immediately halved,
// never going below the configured floor.
func (l *Limiter) ReportRateLimited() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successStreak = 0
	l.capacity = max(l.capacity/2, l.floor)
}

// ReportOther records a failure that was neither a success nor a rate-limit
// signal (a generic timeout, a permanent error, etc.). Capacity is left
// unchanged - a timeout is not a capacity signal - but the success streak is
// reset so a brief run of unrelated errors isn't later miscounted as
// consecutive successes once things recover.
func (l *Limiter) ReportOther() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successStreak = 0
}

// Capacity returns the limiter's current allowed concurrency.
func (l *Limiter) Capacity() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.capacity
}

// InFlight returns the number of currently acquired (not yet released) slots.
func (l *Limiter) InFlight() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inFlight
}

// registry holds one Limiter per AI connector identity, so every review
// sharing a connector's API key draws from the same adaptive capacity instead
// of each review getting its own independent (and therefore uncoordinated)
// limiter. The key is a string rather than a database connector ID because
// callers (internal/batch's BatchTask) don't always have a DB row ID in hand
// at the point they need to gate a call - a stable identity string (e.g.
// "<providerType>:<providerName>") is sufficient: it still gives every
// distinct configured AI connector its own independently-tuned capacity.
var (
	registryMu sync.Mutex
	registry   = map[string]*Limiter{}
)

// ForConnector returns the shared Limiter for connectorKey, creating it with
// cfg on first use. Subsequent calls for the same connectorKey ignore cfg and
// return the existing instance - the limiter's whole point is one shared,
// evolving state per connector, not a fresh one per call.
func ForConnector(connectorKey string, cfg Config) *Limiter {
	registryMu.Lock()
	defer registryMu.Unlock()
	if l, ok := registry[connectorKey]; ok {
		return l
	}
	l := New(cfg)
	registry[connectorKey] = l
	return l
}

// ResetRegistry clears all shared limiters. Exported for tests only.
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]*Limiter{}
}
