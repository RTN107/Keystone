// Package ratelimit holds the sliding-window counter that protects Ask from being
// used as a free model proxy or flooded into the query log.
package ratelimit

import (
	"sync"
	"time"
)

// Window allows at most Max events per key inside Window. It lives in memory,
// which is enough for one instance; the README's Limits section says so.
type Window struct {
	mu     sync.Mutex
	events map[string][]time.Time
	max    int
	window time.Duration
	now    func() time.Time
}

func New(max int, window time.Duration) *Window {
	return &Window{events: map[string][]time.Time{}, max: max, window: window, now: time.Now}
}

// Allow records an event for key and reports whether it is within the limit. When
// it is not, retryAfter says how long until the oldest counted event leaves the
// window. A refused event is not recorded, so waiting is always enough.
func (w *Window) Allow(key string) (ok bool, retryAfter time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	recent := w.prune(key, now)
	if len(recent) >= w.max {
		return false, recent[0].Add(w.window).Sub(now)
	}
	w.events[key] = append(recent, now)
	if len(w.events) > 10000 {
		for k := range w.events {
			w.prune(k, now)
		}
	}
	return true, 0
}

// Peek reports whether an event for key would be allowed, without recording one.
func (w *Window) Peek(key string) (ok bool, retryAfter time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	recent := w.prune(key, now)
	if len(recent) >= w.max {
		return false, recent[0].Add(w.window).Sub(now)
	}
	return true, 0
}

func (w *Window) prune(key string, now time.Time) []time.Time {
	cutoff := now.Add(-w.window)
	kept := w.events[key][:0]
	for _, t := range w.events[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(w.events, key)
		return nil
	}
	w.events[key] = kept
	return kept
}
