package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Throttler emits repeated log events at a bounded rate. The first occurrence
// of a given key always passes through immediately. Subsequent occurrences
// within window are silently counted. On the next emission after the window
// expires, the suppressed count is attached as a "suppressed_since" field so
// the operator can see the line was noisy without the log itself being noisy.
//
// Use case: permission polls, bridge dispatch floods, readLoop errors —
// anywhere a condition fires repeatedly and the rate itself is the signal.
//
// Throttler is safe for concurrent use.
type Throttler struct {
	mu     sync.Mutex
	window time.Duration
	state  map[string]*throttleEntry
}

type throttleEntry struct {
	lastEmit   time.Time
	suppressed int
}

// NewThrottler creates a throttler that admits at most one log per key per
// window. Typical windows are 2s–30s; anything below 1s is effectively a
// no-op given slog's own overhead.
func NewThrottler(window time.Duration) *Throttler {
	return &Throttler{
		window: window,
		state:  map[string]*throttleEntry{},
	}
}

// Log emits through logger iff key has not fired within window. Returns true
// if the line was emitted (caller can suppress follow-up work like stat
// updates). Attrs supply the slog attributes; do not include operation —
// pass it explicitly.
func (t *Throttler) Log(logger *slog.Logger, level slog.Level, key string, msg string, attrs ...any) bool {
	if logger == nil {
		return false
	}
	t.mu.Lock()
	now := time.Now()
	e, ok := t.state[key]
	if !ok {
		e = &throttleEntry{}
		t.state[key] = e
	}
	if ok && now.Sub(e.lastEmit) < t.window {
		e.suppressed++
		t.mu.Unlock()
		return false
	}
	suppressed := e.suppressed
	e.suppressed = 0
	e.lastEmit = now
	t.mu.Unlock()

	if suppressed > 0 {
		attrs = append(attrs, slog.Int("suppressed_since", suppressed))
	}
	logger.Log(context.Background(), level, msg, attrs...)
	return true
}
