package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/testutil"
)

// handlerWithoutDB builds the router with no store. Every request in these tests
// must be answered by middleware before a handler needs the database.
func handlerWithoutDB(readOnlyOn bool) http.Handler {
	return New(Deps{
		Issuer:  auth.NewIssuer(strings.Repeat("s", 32)),
		Limiter: auth.NewLoginLimiter(),
		Config:  config.Config{ReadOnly: readOnlyOn, WebOrigin: "https://app.example"},
		Log:     testutil.Logger(),
	})
}

// TestReadOnlyAllowlistIsExactlyLoginAndAsk pins the allowlist to a literal, so
// widening it is a deliberate edit to this test and not a quiet change to the
// map the other tests read.
func TestReadOnlyAllowlistIsExactlyLoginAndAsk(t *testing.T) {
	want := map[string]bool{"POST /api/auth/login": true, "POST /api/ask": true}
	if len(readOnlyAllowed) != len(want) {
		t.Fatalf("read-only allowlist has %d entries, want %d: %v", len(readOnlyAllowed), len(want), readOnlyAllowed)
	}
	for k := range want {
		if !readOnlyAllowed[k] {
			t.Errorf("allowlist is missing %q", k)
		}
	}
}

// TestReadOnlyRefusesEveryWriteRoute walks the real route list, so a new write
// route is covered the moment it is added.
func TestReadOnlyRefusesEveryWriteRoute(t *testing.T) {
	h := handlerWithoutDB(true)
	checked := 0
	for _, rt := range routes() {
		method, path, _ := strings.Cut(rt.pattern, " ")
		if isSafeMethod(method) || readOnlyAllowed[rt.pattern] {
			continue
		}
		checked++
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"read_only"`) {
			t.Errorf("%s: want 403 read_only, got %d %s", rt.pattern, rec.Code, rec.Body.String())
		}
	}
	if checked == 0 {
		t.Fatal("no write routes were checked; the test is not testing anything")
	}
}

func TestReadOnlyFailsClosedForUnknownWrites(t *testing.T) {
	h := handlerWithoutDB(true)
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, p := range []string{"/api/anything-new", "/api/ask/", "//api/ask", "/api/auth/login/"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(m, p, strings.NewReader(`{}`)))
			if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
				t.Errorf("%s %s succeeded on a read-only deployment", m, p)
			}
		}
	}
}

func TestWritesAreNotBlockedWhenReadOnlyIsOff(t *testing.T) {
	// Unauthenticated admin write: refused by auth (401), not by the read-only guard.
	rec := httptest.NewRecorder()
	handlerWithoutDB(false).ServeHTTP(rec, httptest.NewRequest("POST", "/api/admin/employees", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestCORSAllowsOnlyTheWebOrigin(t *testing.T) {
	h := handlerWithoutDB(false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/me", nil)
	req.Header.Set("Origin", "https://app.example")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("allowed origin: code=%d acao=%q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("OPTIONS", "/api/me", nil)
	req.Header.Set("Origin", "https://elsewhere.example")
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("a foreign origin must get no CORS headers")
	}
}
