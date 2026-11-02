package answer

import (
	"context"
	"errors"
	"strings"

	"keystone/internal/retrieve"
)

// Request is everything sent to the model. A test's fake model records the whole
// value, so a canary check covers the entire prompt.
type Request struct {
	System    string
	User      string
	MaxTokens int
}

// Model streams a completion. onText is called with each piece of text as it
// arrives. The returned stop reason is the model's own ("end_turn", "max_tokens").
type Model interface {
	Stream(ctx context.Context, req Request, onText func(string)) (stopReason string, err error)
}

// Citation ties a [n] marker in the answer to the passage it names.
type Citation struct {
	N       int
	Passage retrieve.Permitted
}

// Result is a finished answer. Text is exactly what was streamed to the client.
type Result struct {
	Text       string
	Citations  []Citation
	Dropped    int  // markers removed because they named no supplied passage
	Truncated  bool // the model hit its length limit
	StopReason string
}

var ErrNoPassages = errors.New("answer: no permitted passages to answer from")

// Generate answers a question from permitted passages only, streaming the text
// through onText as the model writes it. Citations are validated here: a marker
// that names no supplied passage is removed from the stream and never returned.
func Generate(ctx context.Context, m Model, question string, passages []retrieve.Permitted, onText func(string)) (Result, error) {
	if len(passages) == 0 {
		return Result{}, ErrNoPassages
	}
	f := newCitationFilter(len(passages), onText)
	stop, err := m.Stream(ctx, buildRequest(question, passages), f.Write)
	f.Flush()
	res := Result{Text: strings.TrimSpace(f.out.String()), Dropped: f.dropped, StopReason: stop, Truncated: stop == "max_tokens"}
	for _, n := range f.cited {
		res.Citations = append(res.Citations, Citation{N: n, Passage: passages[n-1]})
	}
	return res, err
}
