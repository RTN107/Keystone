package embed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These tests run against a local stand-in for the service. They prove how the
// client behaves, not what the real service returns; the live check is the
// embedcheck tool.

const testKey = "test-key-should-never-appear-in-errors"

// fakeVoyage answers every input with a vector whose first value is the input's
// position within the whole run, so order can be checked end to end.
type fakeVoyage struct {
	dim      int
	requests atomic.Int32
	sizes    []int
	failures atomic.Int32 // how many initial calls to fail with 429
	retryHdr string       // Retry-After to send with the 429; empty sends none, as the real service does
	last     request
}

func (f *fakeVoyage) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.URL.Path != "/embeddings" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"Unauthorized"}`))
			return
		}
		if f.failures.Add(-1) >= 0 {
			if f.retryHdr != "" {
				w.Header().Set("Retry-After", f.retryHdr)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"slow down"}`))
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad body: %v", err)
		}
		f.last = req
		f.sizes = append(f.sizes, len(req.Input))

		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		items := make([]item, len(req.Input))
		// Return out of order to prove the client places by index.
		for i := range req.Input {
			vec := make([]float32, f.dim)
			// encode the input text's numeric tag into the first component
			var tag float32
			for _, c := range req.Input[i] {
				tag = tag*10 + float32(c-'0')
			}
			vec[0] = tag
			items[len(req.Input)-1-i] = item{Embedding: vec, Index: i}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "usage": map[string]int{"total_tokens": len(req.Input) * 3}})
	})
}

func newTestClient(t *testing.T, f *fakeVoyage, dim int) *Client {
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c := New(srv.URL, testKey, dim)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strings.Repeat("", 0) + itoa(i)
	}
	return out
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestEmbedBatchesAndKeepsOrder(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	c := newTestClient(t, f, 4)

	res, err := c.Embed(context.Background(), numbered(130), Document)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vectors) != 130 {
		t.Fatalf("want 130 vectors, got %d", len(res.Vectors))
	}
	for i, v := range res.Vectors {
		if int(v[0]) != i {
			t.Fatalf("vector %d is out of order (tag %v)", i, v[0])
		}
	}
	if got := f.sizes; len(got) != 3 || got[0] != 64 || got[1] != 64 || got[2] != 2 {
		t.Errorf("batch sizes: %v", got)
	}
	if res.Tokens != 130*3 {
		t.Errorf("tokens: %d", res.Tokens)
	}
}

func TestEmbedSendsModelTypeAndPinnedDimension(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	c := newTestClient(t, f, 4)
	if _, err := c.Embed(context.Background(), []string{"1"}, Query); err != nil {
		t.Fatal(err)
	}
	if f.last.Model != "voyage-4-lite" || f.last.InputType != Query || f.last.OutputDimension != 4 {
		t.Errorf("request: %+v", f.last)
	}
}

func TestEmbedWithoutDimensionSendsNoneAndAcceptsAnySize(t *testing.T) {
	f := &fakeVoyage{dim: 7}
	c := newTestClient(t, f, 0)
	res, err := c.Embed(context.Background(), []string{"1"}, Document)
	if err != nil {
		t.Fatal(err)
	}
	if f.last.OutputDimension != 0 || len(res.Vectors[0]) != 7 {
		t.Errorf("sent dim %d, got vector of %d", f.last.OutputDimension, len(res.Vectors[0]))
	}
}

func TestEmbedRefusesWrongDimension(t *testing.T) {
	f := &fakeVoyage{dim: 8}
	c := newTestClient(t, f, 4)
	_, err := c.Embed(context.Background(), []string{"1"}, Document)
	var de *DimensionError
	if !errors.As(err, &de) || de.Got != 8 || de.Want != 4 {
		t.Fatalf("want DimensionError{8,4}, got %v", err)
	}
}

func TestEmbedRetriesRateLimitThenSucceeds(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(2)
	c := newTestClient(t, f, 4)
	if _, err := c.Embed(context.Background(), []string{"1"}, Document); err != nil {
		t.Fatalf("should have succeeded after retries: %v", err)
	}
	if got := f.requests.Load(); got != 3 {
		t.Errorf("want 3 requests (2 limited, 1 ok), got %d", got)
	}
}

func TestEmbedGivesUpAfterFiveRateLimitedAttempts(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(100)
	c := newTestClient(t, f, 4)
	_, err := c.Embed(context.Background(), []string{"1"}, Document)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 429 {
		t.Fatalf("want a 429 APIError, got %v", err)
	}
	if got := f.requests.Load(); got != 5 {
		t.Errorf("want exactly 5 attempts, got %d", got)
	}
}

// The real service sends no Retry-After with its 429. On a capped account the
// window is a minute, so the client must be willing to wait out a full minute.
func TestEmbedOutlastsAOneMinuteWindowWithoutRetryAfter(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(4)
	c := newTestClient(t, f, 4)
	var waited time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waited += d; return nil }
	if _, err := c.Embed(context.Background(), []string{"1"}, Document); err != nil {
		t.Fatalf("should succeed on the fifth attempt: %v", err)
	}
	if waited < 60*time.Second {
		t.Errorf("waited only %v in total, a one minute window would still be closed", waited)
	}
}

func TestEmbedHonoursRetryAfterWhenSent(t *testing.T) {
	f := &fakeVoyage{dim: 4, retryHdr: "7"}
	f.failures.Store(1)
	c := newTestClient(t, f, 4)
	var waits []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	if _, err := c.Embed(context.Background(), []string{"1"}, Document); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != 7*time.Second {
		t.Errorf("want one wait of 7s, got %v", waits)
	}
}

func TestEmbedDoesNotRetryAuthFailureAndNeverLeaksTheKey(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c := New(srv.URL, "a-wrong-key", 4)
	c.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := c.Embed(context.Background(), []string{"1"}, Document)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 401 {
		t.Fatalf("want 401 APIError, got %v", err)
	}
	if f.requests.Load() != 1 {
		t.Errorf("an auth failure must not be retried, got %d requests", f.requests.Load())
	}
	if strings.Contains(err.Error(), "a-wrong-key") || strings.Contains(err.Error(), "Bearer") {
		t.Errorf("error leaks the credential: %v", err)
	}
}

func TestEmbedStopsWhenContextIsCancelled(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(100)
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	c := New(srv.URL, testKey, 4)
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
	if _, err := c.Embed(ctx, []string{"1"}, Document); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestVectorLiteral(t *testing.T) {
	if got := Vector([]float32{0.5, -1, 0.000123}); got != "[0.5,-1,0.000123]" {
		t.Errorf("got %s", got)
	}
	if got := Vector(nil); got != "[]" {
		t.Errorf("got %s", got)
	}
}

func TestBaseURLMayBeTheBaseOrTheFullEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,2,3,4],"index":0}],"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	for _, base := range []string{srv.URL + "/v1", srv.URL + "/v1/", srv.URL + "/v1/embeddings", srv.URL + "/v1/embeddings/", " " + srv.URL + "/v1/embeddings "} {
		c := New(base, testKey, 4)
		if _, err := c.Embed(context.Background(), []string{"1"}, Document); err != nil {
			t.Fatalf("base %q: %v", base, err)
		}
	}
	for i, p := range paths {
		if p != "/v1/embeddings" {
			t.Errorf("call %d went to %q, want /v1/embeddings", i, p)
		}
	}
	if len(paths) != 5 {
		t.Errorf("want 5 calls, got %d", len(paths))
	}
	if got := New("", testKey, 4).baseURL; got != DefaultBaseURL {
		t.Errorf("empty base should fall back to the default, got %q", got)
	}
}

func TestUnreachableServiceIsATypedErrorAndSaysNothingAboutTheKey(t *testing.T) {
	// Connection refused: a server that was started and closed.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := New(url, testKey, 4)
	_, err := c.Embed(context.Background(), []string{"1"}, Document)
	var ue *UnreachableError
	if !errors.As(err, &ue) {
		t.Fatalf("want an UnreachableError for a closed server, got %T %v", err, err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("the key leaked into the error: %v", err)
	}

	// An untrusted certificate (the stand-in's own CA is not in the system store).
	tlsSrv := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsSrv.Close()
	_, err = New(tlsSrv.URL, testKey, 4).Embed(context.Background(), []string{"1"}, Document)
	if !errors.As(err, &ue) {
		t.Fatalf("want an UnreachableError for an untrusted certificate, got %T %v", err, err)
	}
}

func TestACancelledContextIsNotReportedAsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(srv.URL, testKey, 4).Embed(ctx, []string{"1"}, Document)
	var ue *UnreachableError
	if errors.As(err, &ue) || !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled request should surface context.Canceled, got %T %v", err, err)
	}
}

// The notice fires once per pause, before the client sleeps, and not for the
// final refusal that ends the retries, and not for a 5xx.
func TestRateLimitNoticeFiresBeforeEachPause(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(2)
	c := newTestClient(t, f, 4)

	var events []string
	c.sleep = func(_ context.Context, d time.Duration) error {
		events = append(events, "sleep "+d.String())
		return nil
	}
	ctx := WithRateLimitNotice(context.Background(), func(w RateLimitWait) {
		events = append(events, "notice "+itoa(w.Attempt)+"/"+itoa(w.MaxAttempts)+" "+w.Wait.String())
	})
	if _, err := c.Embed(ctx, []string{"1"}, Query); err != nil {
		t.Fatal(err)
	}
	want := []string{"notice 1/5 20s", "sleep 20s", "notice 2/5 20s", "sleep 20s"}
	if strings.Join(events, "|") != strings.Join(want, "|") {
		t.Errorf("events = %v, want %v", events, want)
	}

	// Exhausted retries: four pauses, the fifth refusal ends it with no notice.
	f2 := &fakeVoyage{dim: 4}
	f2.failures.Store(100)
	c2 := newTestClient(t, f2, 4)
	notices := 0
	ctx2 := WithRateLimitNotice(context.Background(), func(RateLimitWait) { notices++ })
	if _, err := c2.Embed(ctx2, []string{"1"}, Query); err == nil {
		t.Fatal("expected the rate limit to win")
	}
	if notices != 4 {
		t.Errorf("want 4 notices (one per pause), got %d", notices)
	}
}

func TestNoNoticeWithoutAHandlerOrForAServerError(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	f.failures.Store(1)
	c := newTestClient(t, f, 4)
	if _, err := c.Embed(context.Background(), []string{"1"}, Query); err != nil {
		t.Fatalf("a context with no handler must still work: %v", err)
	}
}

// Two visitors asking together share one embeddings call at a time. The second waits its
// turn (and is told so) instead of both calling the service and both being refused.
func TestOnlyOneEmbeddingCallRunsAtATimeAndTheWaiterIsToldItIsQueued(t *testing.T) {
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var out struct {
			Data []item `json:"data"`
		}
		for i := range req.Input {
			out.Data = append(out.Data, item{Embedding: make([]float32, 8), Index: i})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	// If the gate is broken, a second call reaches the server and blocks there. Unblock every
	// handler when the test ends so the failure is reported instead of hanging.
	defer func() {
		select {
		case <-release:
		default:
		}
		close(release)
	}()
	c := New(srv.URL, testKey, 8)

	queued := make(chan RateLimitWait, 4)
	notice := func(rl RateLimitWait) { queued <- rl }
	first := make(chan error, 1)
	go func() { _, err := c.Embed(context.Background(), []string{"one"}, Query); first <- err }()
	for inFlight.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	second := make(chan error, 1)
	go func() {
		_, err := c.Embed(WithRateLimitNotice(context.Background(), notice), []string{"two"}, Query)
		second <- err
	}()
	select {
	case rl := <-queued:
		if !rl.Queued {
			t.Error("the waiter should be told it is queued, not rate limited")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second caller was not told it is waiting its turn")
	}
	if inFlight.Load() != 1 {
		t.Fatalf("while one call is in flight the second must not have started: %d in flight", inFlight.Load())
	}
	release <- struct{}{}
	release <- struct{}{}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 1 {
		t.Errorf("peak concurrent embedding calls was %d, want 1", peak.Load())
	}
}

func TestAWaitingCallerThatGivesUpLeavesTheQueue(t *testing.T) {
	c := New("http://127.0.0.1:1", testKey, 8)
	c.gate <- struct{}{} // someone else holds the gate
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Embed(ctx, []string{"x"}, Query); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a caller waiting for the gate must stop when its context ends: %v", err)
	}
}

func TestWithModelChangesTheRequestedModel(t *testing.T) {
	f := &fakeVoyage{dim: 4}
	c := newTestClient(t, f, 4).WithModel("some-other-model")
	if _, err := c.Embed(context.Background(), []string{"a"}, Query); err != nil {
		t.Fatal(err)
	}
	if f.last.Model != "some-other-model" {
		t.Errorf("model sent: %q", f.last.Model)
	}
}
