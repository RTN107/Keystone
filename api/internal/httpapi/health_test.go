package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/store"
	"keystone/internal/testutil"
)

// Health must report answers as available exactly when a key is configured under
// either accepted name, as loaded by the real config path.
func TestHealthAnswersFollowsEitherKeyName(t *testing.T) {
	pool := testutil.DB(t)
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"no key", nil, false},
		{"project name", map[string]string{"KEYSTONE_ANTHROPIC_KEY": "k1"}, true},
		{"fallback name", map[string]string{"ANTHROPIC_API_KEY": "k2"}, true},
	}
	for _, tc := range cases {
		m := map[string]string{"DATABASE_URL": "postgres://u:p@h/db", "JWT_SECRET": strings.Repeat("a", 32), "ANTHROPIC_MODEL": "a-model"}
		for k, v := range tc.env {
			m[k] = v
		}
		cfg, err := config.Load(func(k string) string { return m[k] })
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(New(Deps{
			Store: store.New(pool), Issuer: auth.NewIssuer(strings.Repeat("s", 32)),
			Limiter: auth.NewLoginLimiter(), Config: cfg, Log: testutil.Logger(),
		}))
		h := &harness{t: t, srv: srv}
		code, body := h.do("GET", "/api/health", "", nil)
		srv.Close()
		caps, _ := body["capabilities"].(map[string]any)
		if code != 200 || caps["answers"] != tc.want {
			t.Errorf("%s: answers = %v, want %v (status %d)", tc.name, caps["answers"], tc.want, code)
		}
	}
}
