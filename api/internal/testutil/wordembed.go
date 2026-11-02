package testutil

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"unicode"

	"keystone/internal/embed"
)

// WordEmbedder is a deterministic stand-in for the embeddings service. It hashes
// each word of a text into one of 1024 slots and normalises the result, so two
// texts that share words are close and texts that share none are orthogonal. That
// is enough to put a chosen chunk at the top of a search, which is what the
// access tests need: a forbidden chunk that matches the question perfectly is the
// worst case for a leak.
//
// It records every call so tests can assert how many embedding requests were made.
type WordEmbedder struct {
	mu    sync.Mutex
	Calls []EmbedCall
	// Err, when set, is returned from every call.
	Err error
	// OnCall, when set, runs at the start of every call with the request's context
	// (used to simulate a rate limit notice).
	OnCall func(ctx context.Context)
}

type EmbedCall struct {
	Texts []string
	Kind  embed.InputType
}

func (w *WordEmbedder) Embed(ctx context.Context, texts []string, kind embed.InputType) (embed.Result, error) {
	w.mu.Lock()
	w.Calls = append(w.Calls, EmbedCall{Texts: append([]string(nil), texts...), Kind: kind})
	onCall, err := w.OnCall, w.Err
	w.mu.Unlock()
	if onCall != nil {
		onCall(ctx)
	}
	if err != nil {
		return embed.Result{}, err
	}
	out := embed.Result{Tokens: len(texts)}
	for _, t := range texts {
		out.Vectors = append(out.Vectors, WordVector(t))
	}
	return out, nil
}

// QueryCalls counts calls made with input_type=query.
func (w *WordEmbedder) QueryCalls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.Calls {
		if c.Kind == embed.Query {
			n++
		}
	}
	return n
}

// WordVector is the embedding WordEmbedder gives a text.
func WordVector(text string) []float32 {
	v := make([]float32, embed.Dimension)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		if len(w) < 3 {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%embed.Dimension]++
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		v[0] = 1
		return v
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v
}
