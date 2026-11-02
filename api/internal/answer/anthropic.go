package answer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is Anthropic's API. It is fixed in code on purpose: the app
// never reads a base URL from the environment, so it cannot be pointed at another
// service by a stray variable. Tests use WithBaseURL.
const (
	DefaultBaseURL   = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
)

// APIError is a failed call. Message is Anthropic's own explanation. It never
// contains the API key.
type APIError struct {
	Status  int
	Type    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic returned %d (%s): %s", e.Status, e.Type, e.Message)
}

// Busy reports a rate limit or overload, where asking again shortly may work.
func (e *APIError) Busy() bool {
	return e.Status == http.StatusTooManyRequests || e.Status == 529 || e.Status == http.StatusServiceUnavailable ||
		e.Type == "overloaded_error" || e.Type == "rate_limit_error"
}

// UnreachableError means the request never got an answer.
type UnreachableError struct{ Err error }

func (e *UnreachableError) Error() string { return "anthropic request failed: " + e.Err.Error() }
func (e *UnreachableError) Unwrap() error { return e.Err }

// Client streams completions from the Anthropic Messages API.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewClient(apiKey, model string) *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 60 * time.Second
	// No overall timeout: an answer streams for as long as the model takes, and the
	// request context ends it if the person goes away.
	return &Client{baseURL: DefaultBaseURL, apiKey: apiKey, model: model, http: &http.Client{Transport: tr}}
}

// WithBaseURL points the client elsewhere, for tests.
func (c *Client) WithBaseURL(u string) *Client {
	c.baseURL = strings.TrimRight(u, "/")
	return c
}

func (c *Client) Model() string { return c.model }

type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system"`
	Stream    bool         `json:"stream"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type       string `json:"type"`
		Text       string `json:"text"`
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Stream sends the request and calls onText with each text delta as it arrives.
func (c *Client) Stream(ctx context.Context, req Request, onText func(string)) (string, error) {
	body, err := json.Marshal(apiRequest{
		Model: c.model, MaxTokens: req.MaxTokens, System: req.System, Stream: true,
		Messages: []apiMessage{{Role: "user", Content: req.User}},
	})
	if err != nil {
		return "", err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hr.Header.Set("x-api-key", c.apiKey)
	hr.Header.Set("anthropic-version", anthropicVersion)
	hr.Header.Set("content-type", "application/json")
	hr.Header.Set("accept", "text/event-stream")

	resp, err := c.http.Do(hr)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) && ue.Unwrap() != nil {
			err = ue.Unwrap()
		}
		return "", &UnreachableError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", apiErrorFrom(resp.StatusCode, raw)
	}

	var (
		stop     string
		finished bool
	)
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // "event:" lines, comments and blank separators carry nothing we need
		}
		var ev streamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				onText(ev.Delta.Text)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
		case "message_stop":
			finished = true
		case "error":
			return stop, &APIError{Status: http.StatusBadGateway, Type: ev.Error.Type, Message: trim(ev.Error.Message)}
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return stop, ctx.Err()
		}
		return stop, &UnreachableError{Err: err}
	}
	if !finished {
		if ctx.Err() != nil {
			return stop, ctx.Err()
		}
		return stop, &UnreachableError{Err: errors.New("the stream ended before the answer was complete")}
	}
	return stop, nil
}

func apiErrorFrom(status int, raw []byte) *APIError {
	var v struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	e := &APIError{Status: status}
	if json.Unmarshal(raw, &v) == nil && v.Error.Message != "" {
		e.Type, e.Message = v.Error.Type, trim(v.Error.Message)
	} else {
		e.Message = trim(strings.TrimSpace(string(raw)))
	}
	return e
}

func trim(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
