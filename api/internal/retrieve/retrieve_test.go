package retrieve

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"keystone/internal/ingest"
	"keystone/internal/testutil"
)

func TestDecideIsPlainCodeOverTwoNumbers(t *testing.T) {
	th := Thresholds{Floor: 0.40, Margin: 0.15}
	cases := []struct {
		name       string
		perm, forb float64
		want       Outcome
	}{
		{"clear answer", 0.66, 0.24, Answered},
		{"weak but relevant permitted answer", 0.45, 0.20, Answered},
		{"forbidden far ahead", 0.14, 0.69, Blocked},
		{"forbidden ahead by less than the margin, permitted clears the floor", 0.45, 0.59, Answered},
		{"forbidden ahead by less than the margin, permitted below the floor", 0.30, 0.42, NoMatch},
		{"forbidden strong but permitted just as strong", 0.60, 0.62, Answered},
		{"forbidden beats permitted by a lot but is itself below the floor", 0.05, 0.39, NoMatch},
		{"nothing relevant anywhere", 0.29, 0.28, NoMatch},
		{"no readable documents, forbidden relevant", NoScore, 0.58, Blocked},
		{"no readable documents, forbidden irrelevant", NoScore, 0.20, NoMatch},
		{"nothing forbidden exists", 0.55, NoScore, Answered},
		{"nothing exists at all", NoScore, NoScore, NoMatch},
		{"floor exactly", 0.40, NoScore, Answered},
	}
	for _, c := range cases {
		if got := Decide(th, c.perm, c.forb); got != c.want {
			t.Errorf("%s: Decide(%.2f, %.2f) = %s, want %s", c.name, c.perm, c.forb, got, c.want)
		}
	}
}

// Exactly representable numbers, so the boundary is tested and not float noise.
func TestDecideMarginBoundaryIsInclusive(t *testing.T) {
	th := Thresholds{Floor: 0.5, Margin: 0.25}
	if got := Decide(th, 0.5, 0.75); got != Blocked {
		t.Errorf("ahead by exactly the margin: %s, want blocked", got)
	}
	if got := Decide(th, 0.5, 0.71875); got != Answered {
		t.Errorf("ahead by less than the margin: %s, want answered", got)
	}
	if got := Decide(th, NoScore, 0.5); got != Blocked {
		t.Errorf("forbidden exactly at the floor: %s, want blocked", got)
	}
	if got := Decide(th, NoScore, 0.4999); got != NoMatch {
		t.Errorf("forbidden just under the floor: %s, want no_match", got)
	}
}

func TestQuestionKeyIsNormalised(t *testing.T) {
	a := normalizeQuestion("  How MANY days\tof  annual leave?\n")
	b := normalizeQuestion("how many days of annual leave?")
	if a != b || a != "how many days of annual leave?" {
		t.Errorf("keys differ: %q vs %q", a, b)
	}
}

func TestVectorCacheIsBoundedAndOldestLeavesFirst(t *testing.T) {
	c := newVectorCache(3)
	for _, k := range []string{"a", "b", "c", "d"} {
		c.put(k, []float32{1})
	}
	if c.len() != 3 {
		t.Fatalf("cache holds %d entries, want 3", c.len())
	}
	if _, ok := c.get("a"); ok {
		t.Error("the oldest entry should have been dropped")
	}
	for _, k := range []string{"b", "c", "d"} {
		if _, ok := c.get(k); !ok {
			t.Errorf("%q should still be cached", k)
		}
	}
	// Mutating what get returns must not change the cache.
	v, _ := c.get("b")
	v[0] = 99
	if again, _ := c.get("b"); again[0] != 1 {
		t.Error("the cache handed out its own slice")
	}
}

// Permitted must not be constructible with content outside this package.
func TestPermittedHasNoExportedFields(t *testing.T) {
	typ := reflect.TypeOf(Permitted{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).IsExported() {
			t.Errorf("Permitted.%s is exported; other packages could forge a passage", typ.Field(i).Name)
		}
	}
}

// ---- database tests ----

type world struct {
	pool                       *pgxpool.Pool
	emb                        *testutil.WordEmbedder
	ret                        *Retriever
	sales, delivery, hr, admin int64
	docs                       map[string]int64
}

var testThresholds = Thresholds{Floor: 0.30, Margin: 0.10}

func newWorld(t *testing.T) *world {
	t.Helper()
	pool := testutil.DB(t)
	ctx := context.Background()
	w := &world{pool: pool, emb: &testutil.WordEmbedder{}, docs: map[string]int64{}}
	w.ret = New(pool, w.emb, testThresholds)
	for name, dst := range map[string]*int64{"Sales & Marketing": &w.sales, "Delivery": &w.delivery, "HR": &w.hr} {
		if err := pool.QueryRow(ctx, `SELECT id FROM teams WHERE name = $1`, name).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO employees (employee_number, name, team_id, role, password_hash)
		 VALUES ('ADM-001', 'Admin', NULL, 'admin', 'x') RETURNING id`).Scan(&w.admin); err != nil {
		t.Fatal(err)
	}
	return w
}

// ingest stores a document through the real ingestion service.
func (w *world) ingest(t *testing.T, key, title, body string, teams ...int64) int64 {
	t.Helper()
	svc := ingest.NewService(w.pool, w.emb)
	saved, err := svc.Ingest(context.Background(), ingest.Request{
		Filename: key + ".md", Title: title, Data: []byte("# " + title + "\n\n" + body), TeamIDs: teams, IngestedBy: w.admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	w.docs[key] = saved.DocumentID
	return saved.DocumentID
}

func (w *world) grant(t *testing.T, doc, team int64) {
	t.Helper()
	if _, err := w.pool.Exec(context.Background(),
		`INSERT INTO document_access (document_id, team_id, granted_by, source) VALUES ($1, $2, $3, 'extension')`,
		doc, team, w.admin); err != nil {
		t.Fatal(err)
	}
}

const (
	discountText = "Clients committing to a twelve month retainer receive a retainer discount of ten percent. Retainer discount approval rests with the sales lead."
	leaveText    = "Every permanent employee receives twenty four days of annual leave each calendar year. Annual leave accrues monthly."
	slaText      = "When a priority ticket breaches its response target, page the delivery lead and open an incident bridge."
)

func (w *world) seed(t *testing.T) {
	t.Helper()
	w.ingest(t, "playbook", "Pricing Playbook", "## Retainer discounts\n\n"+discountText, w.sales)
	w.ingest(t, "leave", "Leave Policy", "## Annual leave\n\n"+leaveText, w.hr)
	w.ingest(t, "sop", "Delivery SOP", "## Breach response\n\n"+slaText, w.delivery)
}

func TestSearchOnlyEverReturnsWhatTheTeamMayRead(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	ctx := context.Background()

	// A question that is word for word the forbidden passage: the worst case.
	q := "twelve month retainer discount of ten percent approval sales lead"
	res, err := w.ret.Retrieve(ctx, w.delivery, q)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Blocked {
		t.Fatalf("outcome = %s, want blocked (%s)", res.Outcome, res.Describe())
	}
	if len(res.Passages) != 0 {
		t.Errorf("a blocked result must carry no passages, got %d", len(res.Passages))
	}
	if !reflect.DeepEqual(res.HoldingTeams, []string{"Sales & Marketing"}) {
		t.Errorf("holding teams = %v", res.HoldingTeams)
	}

	// Sales asks the same question and gets the passage.
	res, err = w.ret.Retrieve(ctx, w.sales, q)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Answered || len(res.Passages) == 0 {
		t.Fatalf("sales should be answered: %s", res.Describe())
	}
	if !strings.Contains(res.Passages[0].Text(), "twelve month retainer") {
		t.Errorf("wrong passage first: %q", res.Passages[0].Text())
	}
	for _, p := range res.Passages {
		if p.DocumentID() == w.docs["leave"] || p.DocumentID() == w.docs["sop"] {
			t.Errorf("sales was handed a passage from a document it cannot read: %q", p.DocumentTitle())
		}
	}
}

func TestEveryReturnedPassageBelongsToADocumentTheTeamHolds(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	for team, name := range map[int64]string{w.sales: "Sales & Marketing", w.delivery: "Delivery", w.hr: "HR"} {
		// A query that touches every document.
		got, err := w.ret.search(context.Background(), team, vecLit("retainer discount annual leave priority ticket breaches delivery lead"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("%s: search returned %d passages, want exactly the one it may read", name, len(got))
		}
		for _, p := range got {
			var held bool
			if err := w.pool.QueryRow(context.Background(),
				`SELECT EXISTS (SELECT 1 FROM document_access WHERE document_id = $1 AND team_id = $2)`, p.DocumentID(), team).Scan(&held); err != nil {
				t.Fatal(err)
			}
			if !held {
				t.Errorf("%s was handed a passage from a document it does not hold", name)
			}
		}
	}
}

func TestProbeReturnsOnlyAScoreAndTeamNames(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	// The probe has exactly two outputs; nothing else of a forbidden document
	// can come back through it.
	var _ func(*Retriever, context.Context, int64, string) (float64, []string, error) = (*Retriever).probe
	score, teams, err := w.ret.probe(context.Background(), w.delivery, vecLit("annual leave twenty four days"))
	if err != nil {
		t.Fatal(err)
	}
	if score < 0.3 || !reflect.DeepEqual(teams, []string{"HR"}) {
		t.Errorf("probe = %.3f %v", score, teams)
	}
	// And its SQL names neither the text nor the title column.
	for _, banned := range []string{"c.content", "d.title", "documents"} {
		if strings.Contains(probeSQL, banned) {
			t.Errorf("probe SQL mentions %q", banned)
		}
	}
}

func TestProbeNamesEveryTeamThatHoldsTheDocument(t *testing.T) {
	w := newWorld(t)
	doc := w.ingest(t, "shared", "Shared", "## Mileage\n\nMileage claims are paid at a flat rate per kilometre travelled.", w.sales)
	w.grant(t, doc, w.hr)
	res, err := w.ret.Retrieve(context.Background(), w.delivery, "mileage claims flat rate kilometre travelled")
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Blocked || !reflect.DeepEqual(res.HoldingTeams, []string{"Sales & Marketing", "HR"}) {
		t.Errorf("want blocked by Sales & Marketing and HR, got %s %v", res.Outcome, res.HoldingTeams)
	}
}

func TestAnAccessExtensionTakesEffectOnTheNextAsk(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	ctx := context.Background()
	q := "twelve month retainer discount ten percent"

	first, err := w.ret.Retrieve(ctx, w.delivery, q)
	if err != nil || first.Outcome != Blocked || first.QueryCached {
		t.Fatalf("first ask: %v %s cached=%v", err, first.Outcome, first.QueryCached)
	}
	w.grant(t, w.docs["playbook"], w.delivery)

	// Same question, reworded only by case and spacing: the cache must serve the
	// vector, and the SQL must still see the new grant.
	again, err := w.ret.Retrieve(ctx, w.delivery, "  Twelve MONTH retainer   discount ten percent ")
	if err != nil {
		t.Fatal(err)
	}
	if !again.QueryCached {
		t.Error("the re-ask should reuse the cached vector")
	}
	if again.Outcome != Answered || len(again.Passages) == 0 {
		t.Fatalf("after the extension the re-ask must be answered, got %s", again.Outcome)
	}
	if got := w.emb.QueryCalls(); got != 1 {
		t.Errorf("embedding calls for the two asks = %d, want 1", got)
	}
}

func TestWeakPermittedPassagesAreNotHandedToTheModel(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	// Matches the delivery document well, plus a stray word that is in no document.
	res, err := w.ret.Retrieve(context.Background(), w.delivery, "priority ticket breaches response target page delivery lead incident bridge")
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Answered {
		t.Fatal(res.Describe())
	}
	for _, p := range res.Passages {
		if p.Score() < testThresholds.Floor {
			t.Errorf("a passage scoring %.3f is under the floor %.2f", p.Score(), testThresholds.Floor)
		}
	}
}

func TestNoMatchWhenNothingIsRelevantAnywhere(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	res, err := w.ret.Retrieve(context.Background(), w.delivery, "what is the office wifi password")
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != NoMatch || len(res.Passages) != 0 || len(res.HoldingTeams) != 0 {
		t.Errorf("want a bare no_match, got %s", res.Describe())
	}
}

func TestReadableDocumentUsesTheSamePredicateAsSearch(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	ctx := context.Background()
	docs := NewDocs(w.pool)

	if d, err := docs.Readable(ctx, w.sales, w.docs["playbook"]); err != nil || !strings.Contains(d.Content, "twelve month retainer") {
		t.Errorf("sales should read its own document: %v", err)
	}
	// Delivery cannot read the playbook, and cannot tell it from a missing one.
	_, err := docs.Readable(ctx, w.delivery, w.docs["playbook"])
	if err != ErrNotFound {
		t.Errorf("an unauthorised read must be ErrNotFound, got %v", err)
	}
	_, missing := docs.Readable(ctx, w.delivery, 999999)
	if missing != err {
		t.Errorf("an unauthorised document and a missing one must look the same: %v vs %v", err, missing)
	}
	// Once the access row exists the same call succeeds.
	w.grant(t, w.docs["playbook"], w.delivery)
	if _, err := docs.Readable(ctx, w.delivery, w.docs["playbook"]); err != nil {
		t.Errorf("after the extension delivery should read it: %v", err)
	}
}

func vecLit(text string) string { return vectorLiteral(testutil.WordVector(text)) }

// A warmed question needs no embeddings call when it is first asked, the warm-up is one
// request for all of them, and it stores vectors only: the SQL (and so access) still runs.
func TestWarmingTheCacheSpendsOneRequestAndTheFirstAskSpendsNone(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	ctx := context.Background()
	qs := []string{"twelve month retainer discount ten percent", "office wifi password"}

	if err := w.ret.Warm(ctx, qs); err != nil {
		t.Fatal(err)
	}
	if got := w.emb.QueryCalls(); got != 1 {
		t.Fatalf("warming %d questions made %d embedding calls, want 1", len(qs), got)
	}
	res, err := w.ret.Retrieve(ctx, w.delivery, "  Twelve month RETAINER discount ten percent ")
	if err != nil || !res.QueryCached {
		t.Fatalf("a warmed question should be served from the cache: %v cached=%v", err, res.QueryCached)
	}
	if res.Outcome != Blocked {
		t.Errorf("warming must not widen access: Delivery asking about the playbook is %s, want blocked", res.Outcome)
	}
	if got := w.emb.QueryCalls(); got != 1 {
		t.Errorf("the first ask spent %d more embedding calls, want 0", got-1)
	}
}
