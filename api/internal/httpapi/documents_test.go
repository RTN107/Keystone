package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keystone/internal/auth"
	"keystone/internal/config"
	"keystone/internal/embed"
	"keystone/internal/ingest"
	"keystone/internal/store"
	"keystone/internal/testutil"
)

type stubEmbedder struct {
	err   error
	calls int
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string, _ embed.InputType) (embed.Result, error) {
	s.calls++
	if s.err != nil {
		return embed.Result{}, s.err
	}
	vecs := make([][]float32, len(texts))
	for i := range vecs {
		v := make([]float32, embed.Dimension)
		v[i%embed.Dimension] = 1
		vecs[i] = v
	}
	return embed.Result{Vectors: vecs, Tokens: 5 * len(texts)}, nil
}

type docHarness struct {
	*harness
	team       func(string) int64
	emb        *stubEmbedder
	admin, emp string
}

func newDocHarness(t *testing.T, withIngest bool) *docHarness {
	t.Helper()
	pool := testutil.DB(t)
	emb := &stubEmbedder{}
	deps := Deps{
		Store:   store.New(pool),
		Issuer:  auth.NewIssuer(strings.Repeat("s", 32)),
		Limiter: auth.NewLoginLimiter(),
		Config:  config.Config{WebOrigin: "https://app.example"},
		Log:     testutil.Logger(),
	}
	if withIngest {
		deps.Ingest = ingest.NewService(pool, emb)
	}
	srv := httptest.NewServer(New(deps))
	t.Cleanup(srv.Close)
	h := &harness{t: t, srv: srv}
	team := func(name string) int64 {
		var id int64
		if err := pool.QueryRow(context.Background(), `SELECT id FROM teams WHERE name = $1`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	_, body := h.do("POST", "/api/setup", "", map[string]any{"name": "Admin", "password": "a long enough password"})
	admin := body["token"].(string)
	h.do("POST", "/api/admin/employees", admin, map[string]any{"name": "Asha", "team_id": team("Delivery"), "password": "employee password 1"})
	_, body = h.do("POST", "/api/auth/login", "", map[string]any{"employee_number": "DEL-001", "password": "employee password 1"})
	return &docHarness{harness: h, team: team, emb: emb, admin: admin, emp: body["token"].(string)}
}

func (d *docHarness) upload(token, filename string, data []byte, teamIDs ...int64) (int, map[string]any) {
	d.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if filename != "" {
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write(data)
	}
	for _, id := range teamIDs {
		_ = mw.WriteField("team_id", itoa64(id))
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", d.srv.URL+"/api/admin/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func itoa64(i int64) string { b, _ := json.Marshal(i); return string(b) }

func demo(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "demo-docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUploadListAndLogFlow(t *testing.T) {
	d := newDocHarness(t, true)
	sop := demo(t, "delivery-and-escalation-sop.md")

	code, body := d.upload(d.admin, "delivery-and-escalation-sop.md", sop, d.team("Delivery"))
	if code != 201 {
		t.Fatalf("upload: %d %v", code, body)
	}
	text, err := ingest.Extract("delivery-and-escalation-sop.md", sop)
	if err != nil {
		t.Fatal(err)
	}
	want := len(ingest.Split(text)) // one passage per section, so the count follows the document
	if body["message"] != fmt.Sprintf("%d passages embedded", want) || body["title"] != "Delivery and Escalation SOP" || body["passages"] != float64(want) {
		t.Errorf("result: %v", body)
	}

	code, body = d.do("GET", "/api/admin/documents", d.admin, nil)
	docs, _ := body["documents"].([]any)
	if code != 200 || len(docs) != 1 {
		t.Fatalf("list: %d %v", code, body)
	}
	first := docs[0].(map[string]any)
	teams := first["teams"].([]any)
	if first["passages"] != float64(want) || first["ingested_by"] != "ADM-001" || len(teams) != 1 || teams[0].(map[string]any)["name"] != "Delivery" {
		t.Errorf("document row: %v", first)
	}

	code, body = d.do("GET", "/api/admin/ingestion-log", d.admin, nil)
	entries, _ := body["entries"].([]any)
	if code != 200 || len(entries) != 1 || entries[0].(map[string]any)["by"] != "ADM-001" {
		t.Errorf("log: %d %v", code, body)
	}
}

func TestDocumentRoutesAreAdminOnly(t *testing.T) {
	d := newDocHarness(t, true)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/admin/documents"}, {"GET", "/api/admin/ingestion-log"},
	} {
		if code, _ := d.do(c.method, c.path, d.emp, nil); code != 403 {
			t.Errorf("employee on %s: want 403, got %d", c.path, code)
		}
		if code, _ := d.do(c.method, c.path, "", nil); code != 401 {
			t.Errorf("anonymous on %s: want 401, got %d", c.path, code)
		}
	}
	if code, _ := d.upload(d.emp, "a.md", demo(t, "leave-and-benefits-policy.md"), d.team("HR")); code != 403 {
		t.Errorf("employee upload: want 403, got %d", code)
	}
	if code, _ := d.upload("", "a.md", demo(t, "leave-and-benefits-policy.md"), d.team("HR")); code != 401 {
		t.Errorf("anonymous upload: want 401, got %d", code)
	}
	if d.emb.calls != 0 {
		t.Errorf("a refused upload must not spend an embedding call, got %d", d.emb.calls)
	}
}

func TestUploadRefusalsSayWhatToDo(t *testing.T) {
	d := newDocHarness(t, true)
	hr := d.team("HR")
	data := demo(t, "leave-and-benefits-policy.md")

	check := func(name string, code int, errc string, gotCode int, body map[string]any) {
		t.Helper()
		if gotCode != code || errCode(body) != errc {
			t.Errorf("%s: got %d %v, want %d %s", name, gotCode, body, code, errc)
		}
	}
	c, b := d.upload(d.admin, "", nil, hr)
	check("no file", 400, "no_file", c, b)
	c, b = d.upload(d.admin, "a.md", data)
	check("no team", 400, "no_teams", c, b)
	c, b = d.upload(d.admin, "a.md", data, 9999)
	check("unknown team", 400, "unknown_team", c, b)
	c, b = d.upload(d.admin, "a.docx", data, hr)
	check("unsupported", 415, "unsupported_type", c, b)
	c, b = d.upload(d.admin, "a.txt", []byte("   "), hr)
	check("empty", 422, "unreadable_file", c, b)
	c, b = d.upload(d.admin, "big.txt", bytes.Repeat([]byte("a"), ingest.MaxUploadBytes+1), hr)
	check("too large", 413, "file_too_large", c, b)
	if !strings.Contains(b["error"].(map[string]any)["message"].(string), "5 MB") {
		t.Errorf("size message should name the limit: %v", b)
	}
	c, b = d.upload(d.admin, "exact.txt", bytes.Repeat([]byte("a"), ingest.MaxUploadBytes), hr)
	if c == 413 {
		t.Errorf("a file of exactly 5 MB must be accepted, got %d %v", c, b)
	}
}

func TestEmbeddingFailuresAreExplainedAndStoreNothing(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		phrase string
	}{
		{"rate limited", &embed.APIError{Status: 429, Message: "3 RPM"}, 503, "embeddings_busy", "rate limited"},
		{"bad key", &embed.APIError{Status: 401, Message: "Unauthorized"}, 502, "embeddings_failed", "Nothing was saved"},
		{"wrong size", &embed.DimensionError{Got: 256, Want: 1024}, 502, "embeddings_failed", "Nothing was saved"},
		{"unreachable", &embed.UnreachableError{Err: errors.New("x509: certificate signed by unknown authority")}, 502, "embeddings_unreachable", "VOYAGE_BASE_URL"},
	}
	for _, c := range cases {
		d := newDocHarness(t, true)
		d.emb.err = c.err
		code, body := d.upload(d.admin, "a.md", demo(t, "leave-and-benefits-policy.md"), d.team("HR"))
		msg, _ := body["error"].(map[string]any)["message"].(string)
		if code != c.status || errCode(body) != c.code || !strings.Contains(msg, c.phrase) || !strings.Contains(msg, "Nothing was saved") {
			t.Errorf("%s: %d %v", c.name, code, body)
		}
		if strings.Contains(msg, "3 RPM") || strings.Contains(msg, "Unauthorized") || strings.Contains(msg, "x509") {
			t.Errorf("%s: the provider's own text should not reach the admin: %q", c.name, msg)
		}
		_, list := d.do("GET", "/api/admin/documents", d.admin, nil)
		if docs, _ := list["documents"].([]any); len(docs) != 0 {
			t.Errorf("%s: a failed upload left %d documents", c.name, len(docs))
		}
	}
}

func TestWithoutAnEmbeddingsKeyIngestionSaysSo(t *testing.T) {
	d := newDocHarness(t, false)
	code, body := d.upload(d.admin, "a.md", demo(t, "leave-and-benefits-policy.md"), d.team("HR"))
	if code != 503 || errCode(body) != "embeddings_unavailable" {
		t.Errorf("upload without a key: %d %v", code, body)
	}
	if code, body = d.do("GET", "/api/admin/documents", d.admin, nil); code != 503 || errCode(body) != "embeddings_unavailable" {
		t.Errorf("list without a key: %d %v", code, body)
	}
}

// Every route the router declares as admin-only refuses an anonymous caller (401) and an
// ordinary employee (403), including the query log, which holds every visitor's questions.
func TestEveryAdminRouteRefusesAnonymousAndEmployeeCallers(t *testing.T) {
	d := newDocHarness(t, true)
	fill := strings.NewReplacer("{id}", "1", "{team_id}", "1")
	n := 0
	for _, rt := range routes() {
		if rt.access != adminOnly {
			continue
		}
		method, path, _ := strings.Cut(rt.pattern, " ")
		path = fill.Replace(path)
		n++
		if code, _ := d.do(method, path, "", nil); code != 401 {
			t.Errorf("anonymous %s %s: want 401, got %d", method, path, code)
		}
		if code, _ := d.do(method, path, d.emp, nil); code != 403 {
			t.Errorf("employee %s %s: want 403, got %d", method, path, code)
		}
	}
	if n < 8 {
		t.Errorf("only %d admin routes were checked; the router declares more", n)
	}
}
