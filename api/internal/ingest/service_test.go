package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/jackc/pgx/v5/pgxpool"

	"keystone/internal/embed"
	"keystone/internal/testutil"
)

// fakeEmbedder returns a distinct unit-ish vector per input and records exactly
// what it was asked to embed.
type fakeEmbedder struct {
	dim    int
	inputs []string
	kinds  []embed.InputType
	err    error
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string, kind embed.InputType) (embed.Result, error) {
	if f.err != nil {
		return embed.Result{}, f.err
	}
	f.inputs = append(f.inputs, texts...)
	f.kinds = append(f.kinds, kind)
	dim := f.dim
	if dim == 0 {
		dim = embed.Dimension
	}
	vecs := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, dim)
		v[i%dim] = 1
		vecs[i] = v
	}
	return embed.Result{Vectors: vecs, Tokens: len(texts) * 7}, nil
}

type fixture struct {
	pool                       *pgxpool.Pool
	svc                        *Service
	emb                        *fakeEmbedder
	sales, delivery, hr, admin int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()
	f := &fixture{pool: pool, emb: &fakeEmbedder{}}
	f.svc = NewService(pool, f.emb)
	for name, dst := range map[string]*int64{"Sales & Marketing": &f.sales, "Delivery": &f.delivery, "HR": &f.hr} {
		if err := pool.QueryRow(ctx, `SELECT id FROM teams WHERE name = $1`, name).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO employees (employee_number, name, team_id, role, password_hash)
		 VALUES ('ADM-001', 'Admin', NULL, 'admin', 'x') RETURNING id`).Scan(&f.admin); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func demoFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "demo-docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIngestStoresDocumentAccessAndChunksTogether(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	saved, err := f.svc.Ingest(ctx, Request{
		Filename: "delivery-and-escalation-sop.md", Data: demoFile(t, "delivery-and-escalation-sop.md"),
		TeamIDs: []int64{f.delivery}, IngestedBy: f.admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The expected count follows the document: one passage per section.
	sop, err := Extract("delivery-and-escalation-sop.md", demoFile(t, "delivery-and-escalation-sop.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := len(Split(sop))
	if saved.Title != "Delivery and Escalation SOP" || saved.Passages != want || saved.Message() != fmt.Sprintf("%d passages embedded", want) {
		t.Errorf("unexpected result: %+v %q", saved, saved.Message())
	}
	if len(saved.Teams) != 1 || saved.Teams[0] != "Delivery" {
		t.Errorf("teams: %v", saved.Teams)
	}

	if got := f.count(t, "documents"); got != 1 {
		t.Errorf("documents: %d", got)
	}
	if got := f.count(t, "document_chunks"); got != want {
		t.Errorf("chunks: %d", got)
	}

	var team, grantedBy int64
	var source string
	if err := f.pool.QueryRow(ctx, `SELECT team_id, granted_by, source FROM document_access`).Scan(&team, &grantedBy, &source); err != nil {
		t.Fatal(err)
	}
	if team != f.delivery || grantedBy != f.admin || source != "ingest" {
		t.Errorf("access row: team %d by %d source %q", team, grantedBy, source)
	}

	var dims int
	if err := f.pool.QueryRow(ctx, `SELECT min(vector_dims(embedding)) FROM document_chunks`).Scan(&dims); err != nil || dims != embed.Dimension {
		t.Errorf("stored vector size %d (err %v), want %d", dims, err, embed.Dimension)
	}
}

// The embedding input is the title, a blank line, then the chunk. The stored
// content stays the raw chunk.
func TestEmbedInputCarriesTheTitleButStoredTextDoesNot(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Ingest(context.Background(), Request{
		Filename: "p.md", Data: demoFile(t, "pricing-and-proposal-playbook.md"), TeamIDs: []int64{f.sales}, IngestedBy: f.admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.emb.inputs) == 0 {
		t.Fatal("nothing was embedded")
	}
	for _, k := range f.emb.kinds {
		if k != embed.Document {
			t.Errorf("ingestion must embed with input_type=document, got %q", k)
		}
	}
	rows, err := f.pool.Query(context.Background(), `SELECT chunk_index, content FROM document_chunks ORDER BY chunk_index`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var content string
		if err := rows.Scan(&idx, &content); err != nil {
			t.Fatal(err)
		}
		if want := "Pricing and Proposal Playbook\n\n" + content; f.emb.inputs[idx] != want {
			t.Errorf("chunk %d embed input is not title + blank line + chunk text", idx)
		}
		if strings.HasPrefix(content, "Pricing and Proposal Playbook\n\n") {
			t.Errorf("chunk %d stored content includes the title prefix", idx)
		}
	}
}

// The browser will slice documents.content by these offsets in UTF-16 units. Read
// them back from the database and do exactly that, on text where bytes, runes and
// UTF-16 units all disagree.
func TestStoredOffsetsRoundTripFromTheDatabase(t *testing.T) {
	f := newFixture(t)
	// Built from code points so the source stays ASCII: curly quotes, an emoji
	// (two UTF-16 units) and CJK (three bytes each) make bytes, runes and UTF-16
	// units all differ.
	apos, lq, rq := string(rune(0x2019)), string(rune(0x201C)), string(rune(0x201D))
	emoji, cjk := string(rune(0x1F600)), string([]rune{0x4E2D, 0x6587})
	para := func(n int) string {
		return strings.Repeat("It"+apos+"s a "+emoji+" case "+cjk+" with "+lq+"quotes"+rq+" and more words to fill the line. ", n)
	}
	text := "# Mixed text\n\n" + para(6) + "\n\n## Second\n\n" + para(7) + "\n\n" + para(5)
	saved, err := f.svc.Ingest(context.Background(), Request{Filename: "mixed.md", Data: []byte(text), TeamIDs: []int64{f.hr}, IngestedBy: f.admin})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Passages < 3 {
		t.Fatalf("want several chunks, got %d", saved.Passages)
	}

	var content string
	if err := f.pool.QueryRow(context.Background(), `SELECT content FROM documents`).Scan(&content); err != nil {
		t.Fatal(err)
	}
	units := utf16.Encode([]rune(content))
	rows, err := f.pool.Query(context.Background(), `SELECT content, char_start, char_end FROM document_chunks ORDER BY chunk_index`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var chunk string
		var s, e int
		if err := rows.Scan(&chunk, &s, &e); err != nil {
			t.Fatal(err)
		}
		if got := string(utf16.Decode(units[s:e])); got != chunk {
			t.Errorf("chunk %d: slicing the stored text by the stored offsets gives a different string", n)
		}
		n++
	}
}

func TestEmbeddingFailureStoresNothing(t *testing.T) {
	f := newFixture(t)
	f.emb.err = &embed.APIError{Status: 429, Message: "slow down"}
	_, err := f.svc.Ingest(context.Background(), Request{
		Filename: "a.md", Data: demoFile(t, "leave-and-benefits-policy.md"), TeamIDs: []int64{f.hr}, IngestedBy: f.admin,
	})
	var ae *embed.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want the embedding error back, got %v", err)
	}
	for _, table := range []string{"documents", "document_access", "document_chunks"} {
		if n := f.count(t, table); n != 0 {
			t.Errorf("%s has %d rows after a failed embedding", table, n)
		}
	}
}

// A failure after the document row is written must roll the whole thing back.
// Here the chunks insert fails (the vectors are the wrong size for the column).
func TestFailureMidSaveRollsEverythingBack(t *testing.T) {
	f := newFixture(t)
	f.emb.dim = 3
	_, err := f.svc.Ingest(context.Background(), Request{
		Filename: "a.md", Data: demoFile(t, "leave-and-benefits-policy.md"), TeamIDs: []int64{f.hr}, IngestedBy: f.admin,
	})
	if err == nil {
		t.Fatal("storing 3-dimensional vectors in a vector(1024) column should fail")
	}
	for _, table := range []string{"documents", "document_access", "document_chunks"} {
		if n := f.count(t, table); n != 0 {
			t.Errorf("%s has %d rows after a failed save: the transaction did not roll back", table, n)
		}
	}
}

func TestIngestRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	data := demoFile(t, "leave-and-benefits-policy.md")
	cases := []struct {
		name string
		req  Request
		want error
	}{
		{"no team", Request{Filename: "a.md", Data: data, IngestedBy: f.admin}, ErrNoTeams},
		{"unknown team", Request{Filename: "a.md", Data: data, TeamIDs: []int64{f.hr, 9999}, IngestedBy: f.admin}, ErrUnknownTeam},
		{"unsupported type", Request{Filename: "a.docx", Data: data, TeamIDs: []int64{f.hr}, IngestedBy: f.admin}, ErrUnsupportedType},
		{"empty file", Request{Filename: "a.txt", Data: []byte("  \n "), TeamIDs: []int64{f.hr}, IngestedBy: f.admin}, ErrEmptyDocument},
		{"long title", Request{Filename: "a.md", Title: strings.Repeat("t", 201), Data: data, TeamIDs: []int64{f.hr}, IngestedBy: f.admin}, ErrTitleTooLong},
	}
	for _, c := range cases {
		if _, err := f.svc.Ingest(ctx, c.req); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	if len(f.emb.inputs) != 0 {
		t.Errorf("a refused upload must not spend an embedding call, but %d texts were embedded", len(f.emb.inputs))
	}
	if n := f.count(t, "documents"); n != 0 {
		t.Errorf("documents: %d", n)
	}
}

// The ingestion log is history. An access extension adds a team to the document's
// readers but must not rewrite what was ingested, or to whom.
func TestLogShowsIngestOnlyWhileTheListShowsExtensions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	saved, err := f.svc.Ingest(ctx, Request{
		Filename: "pricing-and-proposal-playbook.md", Data: demoFile(t, "pricing-and-proposal-playbook.md"),
		TeamIDs: []int64{f.sales}, IngestedBy: f.admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO document_access (document_id, team_id, granted_by, source) VALUES ($1, $2, $3, 'extension')`,
		saved.DocumentID, f.delivery, f.admin); err != nil {
		t.Fatal(err)
	}

	log, err := f.svc.IngestionLog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || len(log[0].Teams) != 1 || log[0].Teams[0] != "Sales & Marketing" || log[0].Passages != demoChunkCount(t, "pricing-and-proposal-playbook.md") {
		t.Errorf("log should show the original tag only: %+v", log)
	}
	if log[0].By == nil || *log[0].By != "ADM-001" {
		t.Errorf("log should name who ingested: %+v", log[0].By)
	}

	docs, err := f.svc.Documents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || len(docs[0].Teams) != 2 || docs[0].Passages != demoChunkCount(t, "pricing-and-proposal-playbook.md") {
		t.Fatalf("list should show both teams: %+v", docs)
	}
	var sources []string
	for _, tg := range docs[0].Teams {
		sources = append(sources, tg.Name+"/"+tg.Source)
	}
	if got := strings.Join(sources, ","); got != "Sales & Marketing/ingest,Delivery/extension" && got != "Delivery/extension,Sales & Marketing/ingest" {
		t.Errorf("sources: %s", got)
	}
}

func TestListsAreNewestFirstAndEmptyIsAnEmptyList(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	docs, err := f.svc.Documents(ctx)
	if err != nil || docs == nil || len(docs) != 0 {
		t.Fatalf("empty database: %v %v", docs, err)
	}
	for _, name := range []string{"leave-and-benefits-policy.md", "delivery-and-escalation-sop.md"} {
		if _, err := f.svc.Ingest(ctx, Request{Filename: name, Data: demoFile(t, name), TeamIDs: []int64{f.hr}, IngestedBy: f.admin}); err != nil {
			t.Fatal(err)
		}
	}
	docs, _ = f.svc.Documents(ctx)
	if len(docs) != 2 || docs[0].Title != "Delivery and Escalation SOP" {
		t.Errorf("newest should be first: %+v", docs)
	}
	log, _ := f.svc.IngestionLog(ctx)
	if len(log) != 2 || log[0].Title != "Delivery and Escalation SOP" {
		t.Errorf("log newest first: %+v", log)
	}
}

func TestDeriveTitle(t *testing.T) {
	cases := []struct{ file, text, want string }{
		{"x.md", "# Real Title\n\nbody", "Real Title"},
		{"x.md", "intro\n\n# Later Title  \n", "Later Title"},
		{"leave_policy-2026.txt", "no heading here", "leave policy 2026"},
		{"## only second level.md", "## nope", "## only second level"},
		{"", "", "Untitled document"},
	}
	for _, c := range cases {
		if got := DeriveTitle(c.file, c.text); got != c.want {
			t.Errorf("DeriveTitle(%q, %q) = %q, want %q", c.file, c.text, got, c.want)
		}
	}
}

// demoChunkCount is the number of passages a demo document splits into: one per
// section, so tests follow the document instead of a remembered number.
func demoChunkCount(t *testing.T, file string) int {
	t.Helper()
	text, err := Extract(file, demoFile(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return len(Split(text))
}

// Deleting a document removes its passages and its access rows with it, counts what it removed,
// and leaves other documents and their passages alone.
func TestDeleteDocumentRemovesItsPassagesAndAccessAndNothingElse(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, err := f.svc.Ingest(ctx, Request{Filename: "a.md", Data: []byte("# A\n\n## One\n\nFirst section text.\n\n## Two\n\nSecond section text."), TeamIDs: []int64{f.hr}, IngestedBy: f.admin})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Ingest(ctx, Request{Filename: "b.md", Data: []byte("# B\n\n## Only\n\nOther document text."), TeamIDs: []int64{f.delivery}, IngestedBy: f.admin})
	if err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.DeleteDocument(ctx, a.DocumentID)
	if err != nil || d.Passages == 0 || d.Title == "" {
		t.Fatalf("delete: %+v %v", d, err)
	}
	var chunks, access, other int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM document_chunks WHERE document_id = $1`, a.DocumentID).Scan(&chunks)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM document_access WHERE document_id = $1`, a.DocumentID).Scan(&access)
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM document_chunks WHERE document_id = $1`, b.DocumentID).Scan(&other)
	if chunks != 0 || access != 0 {
		t.Errorf("passages and access rows must go with the document: %d chunks, %d access rows", chunks, access)
	}
	if other == 0 {
		t.Error("the other document's passages must be untouched")
	}
	if _, err := f.svc.DeleteDocument(ctx, a.DocumentID); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("deleting again: want ErrDocumentNotFound, got %v", err)
	}
}
