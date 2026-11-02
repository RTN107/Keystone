package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// FakeVoyage is a local stand-in for the embeddings HTTP API. It answers with
// WordVector embeddings and can answer 429 (with no Retry-After, like the real
// service) a chosen number of times first. Pair it with a real embed.Client so the
// real request, retry and parsing code runs.
type FakeVoyage struct {
	URL      string
	Requests atomic.Int32
	// RateLimited is how many requests, counting from now, are answered 429.
	RateLimited atomic.Int32
}

func NewFakeVoyage(t *testing.T) *FakeVoyage {
	t.Helper()
	f := &FakeVoyage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.Requests.Add(1)
		if f.RateLimited.Add(-1) >= 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"You have exceeded the rate limit."}`))
			return
		}
		var in struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		items := make([]item, len(in.Input))
		for i, text := range in.Input {
			items[i] = item{WordVector(text), i}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "usage": map[string]int{"total_tokens": len(in.Input)}})
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}
