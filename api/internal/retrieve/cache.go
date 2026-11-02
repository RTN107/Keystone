package retrieve

import (
	"strings"
	"sync"
)

// DefaultCacheEntries bounds the query-vector cache.
const DefaultCacheEntries = 256

// vectorCache remembers the embedding of a question, keyed by the question's
// normalised text. It holds the VECTOR only. It never holds results, scores,
// passages or anything about access, so a changed grant takes effect on the very
// next ask: the SQL runs again every time, only the embedding call is skipped.
// The oldest entry is dropped first when the cache is full.
type vectorCache struct {
	mu    sync.Mutex
	max   int
	vecs  map[string][]float32
	order []string // insertion order, oldest first
}

func newVectorCache(max int) *vectorCache {
	return &vectorCache{max: max, vecs: make(map[string][]float32, max)}
}

// normalizeQuestion makes "  Annual  LEAVE? " and "annual leave?" the same key:
// trimmed, lowercased, runs of whitespace collapsed to one space.
func normalizeQuestion(q string) string {
	return strings.ToLower(strings.Join(strings.Fields(q), " "))
}

func (c *vectorCache) get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.vecs[key]
	if !ok {
		return nil, false
	}
	return append([]float32(nil), v...), true
}

func (c *vectorCache) put(key string, v []float32) {
	if c.max <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.vecs[key]; !ok {
		if len(c.order) >= c.max {
			delete(c.vecs, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, key)
	}
	c.vecs[key] = append([]float32(nil), v...)
}

func (c *vectorCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.vecs)
}
