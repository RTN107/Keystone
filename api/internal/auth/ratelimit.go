package auth

import (
	"sync"
	"time"
)

// LoginLimiter blocks a key (client address plus employee number) after too many
// failed sign-ins inside a window. It lives in memory, which is enough for one
// instance; the Limits section of the README says so.
type LoginLimiter struct {
	mu     sync.Mutex
	fails  map[string][]time.Time
	max    int
	window time.Duration
	now    func() time.Time
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{fails: map[string][]time.Time{}, max: 5, window: 15 * time.Minute, now: time.Now}
}

// Allow reports whether the key may attempt a sign-in.
func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key)) < l.max
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.recent(key), l.now())
	if len(l.fails) > 10000 {
		for k := range l.fails {
			if len(l.recent(k)) == 0 {
				delete(l.fails, k)
			}
		}
	}
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

// recent drops expired failures for key and returns what is left. Callers hold mu.
func (l *LoginLimiter) recent(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	kept := l.fails[key][:0]
	for _, t := range l.fails[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = kept
	return kept
}
