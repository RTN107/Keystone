package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordRules(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password should be refused")
	}
	if ValidatePassword(strings.Repeat("a", 73)) == nil {
		t.Error("password over 72 bytes should be refused, not silently truncated")
	}
	if err := ValidatePassword("long enough pw"); err != nil {
		t.Errorf("valid password refused: %v", err)
	}
	h, err := HashPassword("long enough pw")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "long enough pw") || CheckPassword(h, "wrong password") {
		t.Error("hash check wrong")
	}
}

func TestTokenRoundTripExpiryAndTamper(t *testing.T) {
	now := time.Now()
	iss := NewIssuer(strings.Repeat("s", 32))
	iss.now = func() time.Time { return now }

	tok, _, err := iss.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := iss.Parse(tok); err != nil || id != 42 {
		t.Fatalf("round trip: id=%d err=%v", id, err)
	}

	if _, err := iss.Parse(tok + "x"); err == nil {
		t.Error("tampered token accepted")
	}
	other := NewIssuer(strings.Repeat("z", 32))
	if _, err := other.Parse(tok); err == nil {
		t.Error("token signed with a different secret accepted")
	}

	iss.now = func() time.Time { return now.Add(TokenTTL + time.Minute) }
	if _, err := iss.Parse(tok); err == nil {
		t.Error("expired token accepted")
	}
}

func TestLimiterBlocksThenExpires(t *testing.T) {
	now := time.Now()
	l := NewLoginLimiter()
	l.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if !l.Allow("k") {
			t.Fatalf("blocked too early at failure %d", i)
		}
		l.Fail("k")
	}
	if l.Allow("k") {
		t.Fatal("should be blocked after 5 failures")
	}
	if !l.Allow("other") {
		t.Fatal("a different key must be unaffected")
	}
	now = now.Add(16 * time.Minute)
	if !l.Allow("k") {
		t.Fatal("should be allowed after the window")
	}
	l.Fail("k")
	l.Reset("k")
	if !l.Allow("k") {
		t.Fatal("reset should clear failures")
	}
}

func TestAuthenticateRefusesBadTokens(t *testing.T) {
	iss := NewIssuer(strings.Repeat("s", 32))
	lookup := func(ctx context.Context, id int64) (Principal, error) {
		if id == 1 {
			return Principal{ID: 1, Role: RoleEmployee}, nil
		}
		return Principal{}, ErrUnknownEmployee
	}
	var status int
	fail := func(w http.ResponseWriter, s int, code, msg string) { status = s; w.WriteHeader(s) }
	reached := false
	h := Authenticate(iss, lookup, fail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if p, ok := PrincipalFrom(r.Context()); !ok || p.ID != 1 {
			t.Error("principal missing from context")
		}
	}))

	call := func(header string) {
		reached, status = false, 0
		req := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	call("")
	if reached || status != 401 {
		t.Error("missing header must give 401")
	}
	call("Bearer nonsense")
	if reached || status != 401 {
		t.Error("garbage token must give 401")
	}
	gone, _, _ := iss.Issue(99)
	call("Bearer " + gone)
	if reached || status != 401 {
		t.Error("token for a deleted employee must give 401")
	}
	good, _, _ := iss.Issue(1)
	call("Bearer " + good)
	if !reached {
		t.Error("valid token should reach the handler")
	}
}
