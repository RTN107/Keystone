// Package embed is the client for the Voyage embeddings API, served through
// MongoDB's endpoint. It batches, retries transient failures, and refuses a
// response whose vector size is not the one the database column expects.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	Model = "voyage-4-lite"
	// Dimension is the size of every stored passage vector. It was
	// confirmed against the live service, and every call pins it.
	Dimension      = 1024
	DefaultBaseURL = "https://ai.mongodb.com/v1"

	// maxBatch keeps each request far under the API's input and token limits.
	maxBatch    = 64
	maxAttempts = 4

	// An account without a payment method is limited to 3 requests a minute and the
	// service sends no Retry-After with the 429, so the client has no hint. Waiting
	// 20 seconds between up to 5 attempts (80 seconds in all) outlasts the one minute
	// window, so a capped account is slow, never failed.
	rateLimitWait        = 20 * time.Second
	maxRateLimitAttempts = 5
)

type InputType string

const (
	// Document is for text that is stored and searched.
	Document InputType = "document"
	// Query is for a question being searched with.
	Query InputType = "query"
)

// RateLimitWait describes a pause the client is about to take because the service
// answered 429. Attempt is the attempt that was just refused, out of MaxAttempts.
type RateLimitWait struct {
	Attempt     int
	MaxAttempts int
	Wait        time.Duration
	// Queued is true when the pause is because another embedding call is in flight and this one
	// is behind it, not because the service refused this call.
	Queued bool
}

type noticeKey struct{}

// WithRateLimitNotice returns a context whose embedding calls report each
// rate-limit pause to fn, just before the client sleeps. fn runs on the calling
// goroutine, so a request handler can write to its own response from it. This is
// how the Ask stream tells the person it is waiting instead of going silent.
func WithRateLimitNotice(ctx context.Context, fn func(RateLimitWait)) context.Context {
	return context.WithValue(ctx, noticeKey{}, fn)
}

// APIError is a failed call. Message is the service's own explanation, trimmed.
// It never contains the API key.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("embeddings service returned %d: %s", e.Status, e.Message)
}

// UnreachableError means the request never got an answer: the host could not be
// found or reached, the connection failed, or its certificate was not trusted. A
// retry will not help, so the caller should tell the admin to check the base URL
// and the network. The message never contains the API key.
type UnreachableError struct{ Err error }

func (e *UnreachableError) Error() string { return "embeddings request failed: " + e.Err.Error() }
func (e *UnreachableError) Unwrap() error { return e.Err }

// DimensionError means the service returned vectors of a different size than the
// database column holds. Storing them would fail later and far from the cause.
type DimensionError struct{ Got, Want int }

func (e *DimensionError) Error() string {
	return fmt.Sprintf("embeddings service returned %d-dimensional vectors, expected %d", e.Got, e.Want)
}

type Client struct {
	baseURL   string
	apiKey    string
	dimension int // 0 means: do not request a size and do not validate one
	model     string
	http      *http.Client
	sleep     func(context.Context, time.Duration) error

	rateLimitWait time.Duration
	// gate lets one embedding call run at a time. The account allows a handful of requests
	// a minute, so when several visitors ask together they queue here, each behind the one in
	// front, instead of all being refused and all retrying at the same moment. The gate is
	// held across the rate-limit waits, so the queue moves on once the call in front succeeds.
	gate chan struct{}
}

// New builds a client. A dimension of 0 asks for the model's default and skips
// validation, which only the embedcheck tool uses.
func New(baseURL, apiKey string, dimension int) *Client {
	return &Client{
		baseURL:   normalizeBaseURL(baseURL),
		apiKey:    apiKey,
		dimension: dimension,
		model:     Model,
		http:      &http.Client{Timeout: 60 * time.Second},
		sleep:     sleepCtx,

		rateLimitWait: rateLimitWait,
		gate:          make(chan struct{}, 1),
	}
}

// WithModel changes which embedding model is requested. The default is Model. Any model used
// here must be able to return vectors of the size the client was built with.
func (c *Client) WithModel(name string) *Client {
	c.model = name
	return c
}

// WithRateLimitWait changes how long the client waits after a 429. Tests use it to
// exercise the real retry path in milliseconds; production keeps the default.
func (c *Client) WithRateLimitWait(d time.Duration) *Client {
	c.rateLimitWait = d
	return c
}

// Endpoint is the URL embedding calls are sent to, for diagnostics.
func (c *Client) Endpoint() string { return c.baseURL + "/embeddings" }

// EmbedWithDimension is Embed with a different requested size. The embedcheck tool
// uses it to learn which sizes the service honours.
func (c *Client) EmbedWithDimension(ctx context.Context, texts []string, kind InputType, dimension int) (Result, error) {
	clone := *c
	clone.dimension = dimension
	return clone.Embed(ctx, texts, kind)
}

// normalizeBaseURL accepts either the API base (https://host/v1) or the full
// endpoint (https://host/v1/embeddings), with or without a trailing slash, and
// returns the base. Both forms are natural to paste into VOYAGE_BASE_URL, and
// treating the full endpoint as a base would call .../embeddings/embeddings.
func normalizeBaseURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	u = strings.TrimSuffix(u, "/embeddings")
	if u == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(u, "/")
}

type request struct {
	Input           []string  `json:"input"`
	Model           string    `json:"model"`
	InputType       InputType `json:"input_type"`
	OutputDimension int       `json:"output_dimension,omitempty"`
}

type response struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Result carries vectors in the same order as the input texts.
type Result struct {
	Vectors [][]float32
	Tokens  int
}

// Embed returns one vector per text, in order, splitting into batches as needed.
func (c *Client) Embed(ctx context.Context, texts []string, kind InputType) (Result, error) {
	release, err := c.acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	defer release()
	var out Result
	for start := 0; start < len(texts); start += maxBatch {
		end := min(start+maxBatch, len(texts))
		vecs, tokens, err := c.embedBatch(ctx, texts[start:end], kind)
		if err != nil {
			return Result{}, err
		}
		out.Vectors = append(out.Vectors, vecs...)
		out.Tokens += tokens
	}
	return out, nil
}

// acquire takes the gate. If another call holds it, the caller is told once (through the
// same notice a rate-limit wait uses) that it is waiting its turn, then waits for it.
func (c *Client) acquire(ctx context.Context) (func(), error) {
	if c.gate == nil {
		return func() {}, nil
	}
	release := func() { <-c.gate }
	select {
	case c.gate <- struct{}{}:
		return release, nil
	default:
	}
	if fn, ok := ctx.Value(noticeKey{}).(func(RateLimitWait)); ok && fn != nil {
		fn(RateLimitWait{Attempt: 1, MaxAttempts: maxRateLimitAttempts, Wait: c.rateLimitWait, Queued: true})
	}
	select {
	case c.gate <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) embedBatch(ctx context.Context, texts []string, kind InputType) ([][]float32, int, error) {
	body, err := json.Marshal(request{Input: texts, Model: c.model, InputType: kind, OutputDimension: c.dimension})
	if err != nil {
		return nil, 0, err
	}

	delay := time.Second
	for attempt := 1; ; attempt++ {
		vecs, tokens, retryAfter, err := c.once(ctx, body, len(texts))
		if err == nil {
			return vecs, tokens, nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !retryable(apiErr.Status) {
			return nil, 0, err
		}
		limit, wait := maxAttempts, delay
		if apiErr.Status == http.StatusTooManyRequests {
			limit, wait = maxRateLimitAttempts, c.rateLimitWait
		}
		if attempt >= limit {
			return nil, 0, err
		}
		if retryAfter > 0 {
			wait = retryAfter
		}
		if apiErr.Status == http.StatusTooManyRequests {
			if fn, ok := ctx.Value(noticeKey{}).(func(RateLimitWait)); ok && fn != nil {
				fn(RateLimitWait{Attempt: attempt, MaxAttempts: limit, Wait: wait})
			}
		}
		if err := c.sleep(ctx, wait); err != nil {
			return nil, 0, err
		}
		delay *= 2
	}
}

func retryable(status int) bool { return status == http.StatusTooManyRequests || status >= 500 }

func (c *Client) once(ctx context.Context, body []byte, want int) ([][]float32, int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// A cancelled or timed out context is the caller's doing, not the service's.
		if ctx.Err() != nil {
			return nil, 0, 0, ctx.Err()
		}
		// The transport error names the URL, never the key, but flatten it anyway.
		return nil, 0, 0, &UnreachableError{Err: unwrapURLError(err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, 0, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, retryAfter(resp), &APIError{Status: resp.StatusCode, Message: serviceMessage(raw)}
	}

	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, 0, 0, fmt.Errorf("embeddings response was not valid JSON: %w", err)
	}
	if len(parsed.Data) != want {
		return nil, 0, 0, fmt.Errorf("embeddings service returned %d vectors for %d inputs", len(parsed.Data), want)
	}
	vecs := make([][]float32, want)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= want || vecs[d.Index] != nil {
			return nil, 0, 0, fmt.Errorf("embeddings response had an unexpected index %d", d.Index)
		}
		if c.dimension != 0 && len(d.Embedding) != c.dimension {
			return nil, 0, 0, &DimensionError{Got: len(d.Embedding), Want: c.dimension}
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, parsed.Usage.TotalTokens, 0, nil
}

func unwrapURLError(err error) error {
	var ue interface{ Unwrap() error }
	if errors.As(err, &ue) && ue.Unwrap() != nil {
		return ue.Unwrap()
	}
	return err
}

func retryAfter(resp *http.Response) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s <= 90 {
		return time.Duration(s) * time.Second
	}
	return 0
}

// serviceMessage pulls the service's own explanation out of an error body.
func serviceMessage(raw []byte) string {
	var v struct {
		Detail  any    `json:"detail"`
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &v) == nil {
		switch d := v.Detail.(type) {
		case string:
			msg = d
		case nil:
			if v.Message != "" {
				msg = v.Message
			}
		}
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Vector renders a vector as a pgvector literal, for example "[0.1,0.2]". It is
// passed as text and cast with ::vector, which works under the simple protocol.
func Vector(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
