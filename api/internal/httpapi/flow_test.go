package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/store"
	"keystone/internal/testutil"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
}

func newHarness(t *testing.T) (*harness, func(string) int64) {
	t.Helper()
	pool := testutil.DB(t)
	h := New(Deps{
		Store:   store.New(pool),
		Issuer:  auth.NewIssuer(strings.Repeat("s", 32)),
		Limiter: auth.NewLoginLimiter(),
		Config:  config.Config{WebOrigin: "https://app.example"},
		Log:     testutil.Logger(),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	teamID := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(context.Background(), `SELECT id FROM teams WHERE name = $1`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	return &harness{t: t, srv: srv}, teamID
}

func (h *harness) do(method, path, token string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestSetupLoginAndEmployeeFlow(t *testing.T) {
	h, team := newHarness(t)

	code, body := h.do("GET", "/api/health", "", nil)
	if code != 200 || body["needs_setup"] != true {
		t.Fatalf("fresh database should need setup: %d %v", code, body)
	}

	if code, body = h.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "short"}); code != 400 || errCode(body) != "invalid_password" {
		t.Errorf("short password on setup: %d %v", code, body)
	}
	code, body = h.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "a long enough password"})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, body)
	}
	adminToken := body["token"].(string)
	if n := body["employee"].(map[string]any)["employee_number"]; n != "ADM-001" {
		t.Errorf("first admin number: %v", n)
	}
	if code, body = h.do("POST", "/api/setup", "", map[string]any{"name": "Second", "password": "another long password"}); code != 409 || errCode(body) != "already_set_up" {
		t.Errorf("second setup must be refused: %d %v", code, body)
	}
	if _, body = h.do("GET", "/api/health", "", nil); body["needs_setup"] != false {
		t.Error("needs_setup should be false after setup")
	}

	// Employees: auto numbers per team, explicit numbers, duplicates, bad shapes.
	del := team("Delivery")
	mk := func(name, number string) (int, map[string]any) {
		return h.do("POST", "/api/admin/employees", adminToken, map[string]any{"name": name, "team_id": del, "employee_number": number, "password": "employee password 1"})
	}
	if code, body = mk("Asha", ""); code != 201 || body["employee_number"] != "DEL-001" {
		t.Fatalf("first delivery employee: %d %v", code, body)
	}
	if code, body = mk("Ravi", ""); code != 201 || body["employee_number"] != "DEL-002" {
		t.Errorf("second delivery employee: %d %v", code, body)
	}
	if code, body = mk("Dup", "del-001"); code != 409 || errCode(body) != "number_taken" {
		t.Errorf("duplicate number: %d %v", code, body)
	}
	if code, body = mk("Bad", "NOPE"); code != 400 || errCode(body) != "invalid_number" {
		t.Errorf("malformed number: %d %v", code, body)
	}
	if code, body = h.do("POST", "/api/admin/employees", adminToken, map[string]any{"name": "X", "team_id": 9999, "password": "employee password 1"}); code != 400 || errCode(body) != "unknown_team" {
		t.Errorf("unknown team: %d %v", code, body)
	}

	// Login: case-insensitive number, wrong password, and the role boundary.
	code, body = h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": " del-001 ", "password": "employee password 1"})
	if code != 200 {
		t.Fatalf("employee login: %d %v", code, body)
	}
	empToken := body["token"].(string)
	if team := body["employee"].(map[string]any)["team"].(map[string]any); team["name"] != "Delivery" {
		t.Errorf("employee team: %v", team)
	}
	if code, body = h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": "DEL-001", "password": "not the password"}); code != 401 || errCode(body) != "invalid_credentials" {
		t.Errorf("wrong password: %d %v", code, body)
	}
	if code, body = h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": "NOPE-999", "password": "whatever long"}); code != 401 || errCode(body) != "invalid_credentials" {
		t.Errorf("unknown number must look the same as a wrong password: %d %v", code, body)
	}

	if code, _ = h.do("GET", "/api/admin/teams", empToken, nil); code != 403 {
		t.Errorf("employee on admin route: want 403, got %d", code)
	}
	if code, _ = h.do("POST", "/api/admin/employees", empToken, map[string]any{"name": "X", "team_id": del, "password": "employee password 1"}); code != 403 {
		t.Errorf("employee creating employee: want 403, got %d", code)
	}
	if code, _ = h.do("GET", "/api/admin/teams", "", nil); code != 401 {
		t.Errorf("anonymous on admin route: want 401, got %d", code)
	}
	code, body = h.do("GET", "/api/admin/teams", adminToken, nil)
	if code != 200 {
		t.Fatalf("admin teams: %d %v", code, body)
	}
	teams := body["teams"].([]any)
	if len(teams) != 3 {
		t.Fatalf("want 3 teams, got %d", len(teams))
	}
	for _, tm := range teams {
		tm := tm.(map[string]any)
		if tm["name"] == "Delivery" && len(tm["members"].([]any)) != 2 {
			t.Errorf("delivery members: %v", tm["members"])
		}
	}

	if code, body = h.do("GET", "/api/me", empToken, nil); code != 200 || body["employee_number"] != "DEL-001" {
		t.Errorf("me: %d %v", code, body)
	}
}

func TestSetupRaceHasExactlyOneWinner(t *testing.T) {
	h, _ := newHarness(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, conflicts := 0, 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := h.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "a long enough password"})
			mu.Lock()
			defer mu.Unlock()
			switch code {
			case 201:
				wins++
			case 409:
				conflicts++
			}
		}()
	}
	wg.Wait()
	if wins != 1 || conflicts != 5 {
		t.Errorf("want 1 winner and 5 conflicts, got %d and %d", wins, conflicts)
	}
}

func TestLoginIsThrottledAfterRepeatedFailures(t *testing.T) {
	h, _ := newHarness(t)
	h.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "a long enough password"})
	for i := 0; i < 5; i++ {
		if code, _ := h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": "ADM-001", "password": "wrong password!"}); code != 401 {
			t.Fatalf("attempt %d: want 401, got %d", i, code)
		}
	}
	code, body := h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": "ADM-001", "password": "a long enough password"})
	if code != 429 || errCode(body) != "too_many_attempts" {
		t.Errorf("want throttled, got %d %v", code, body)
	}
}

func TestTokenDoesNotOutliveTheEmployee(t *testing.T) {
	pool := testutil.DB(t)
	h := New(Deps{Store: store.New(pool), Issuer: auth.NewIssuer(strings.Repeat("s", 32)), Limiter: auth.NewLoginLimiter(),
		Config: config.Config{WebOrigin: "https://app.example"}, Log: testutil.Logger()})
	srv := httptest.NewServer(h)
	defer srv.Close()
	hh := &harness{t: t, srv: srv}

	_, body := hh.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "a long enough password"})
	token := body["token"].(string)
	if code, _ := hh.do("GET", "/api/me", token, nil); code != 200 {
		t.Fatalf("me before delete: %d", code)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM employees`); err != nil {
		t.Fatal(err)
	}
	if code, _ := hh.do("GET", "/api/me", token, nil); code != 401 {
		t.Errorf("token for a deleted employee: want 401, got %d", code)
	}
}

func TestSchemaRefusesAdminWithTeamAndEmployeeWithout(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO employees (employee_number, name, team_id, role, password_hash) VALUES ('ADM-009','A',1,'admin','x')`); err == nil {
		t.Error("an admin with a team must be refused by the schema")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO employees (employee_number, name, team_id, role, password_hash) VALUES ('DEL-009','E',NULL,'employee','x')`); err == nil {
		t.Error("an employee without a team must be refused by the schema")
	}
}
