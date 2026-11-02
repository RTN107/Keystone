package ingest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"keystone/internal/embed"
)

// TestLiveDemoRetrieval is a diagnostic, not part of the product. It ingests the
// three demo documents through the real service with REAL embeddings, then asks
// each demo question and reports which passage ranks first and how far ahead of
// the rest it is. It spends embedding calls, so it only runs when asked:
//
//	LIVE_EMBEDDINGS=1 VOYAGE_API_KEY=... TEST_DATABASE_URL=... \
//	  go test -p 1 -count=1 -timeout 20m -run TestLiveDemoRetrieval -v ./internal/ingest
//
// The ranking here is global (no team constraint) so it shows what the embeddings
// themselves think. The team-scoped query that answers questions is in package retrieve.
func TestLiveDemoRetrieval(t *testing.T) {
	if os.Getenv("LIVE_EMBEDDINGS") != "1" || os.Getenv("VOYAGE_API_KEY") == "" {
		t.Skip("set LIVE_EMBEDDINGS=1 and VOYAGE_API_KEY to run against the real embeddings service")
	}
	f := newFixture(t)
	client := embed.New(os.Getenv("VOYAGE_BASE_URL"), os.Getenv("VOYAGE_API_KEY"), embed.Dimension)
	f.svc = NewService(f.pool, client)
	ctx := context.Background()

	// Which team holds which document, as in the demo.
	holder := map[string]string{
		"pricing-and-proposal-playbook.md": "Sales & Marketing",
		"delivery-and-escalation-sop.md":   "Delivery",
		"leave-and-benefits-policy.md":     "HR",
	}
	teamID := map[string]int64{"Sales & Marketing": f.sales, "Delivery": f.delivery, "HR": f.hr}

	start := time.Now()
	for _, file := range []string{"pricing-and-proposal-playbook.md", "leave-and-benefits-policy.md", "delivery-and-escalation-sop.md"} {
		at := time.Now()
		saved, err := f.svc.Ingest(ctx, Request{Filename: file, Data: demoFile(t, file), TeamIDs: []int64{teamID[holder[file]]}, IngestedBy: f.admin})
		if err != nil {
			t.Fatalf("ingest %s: %v", file, err)
		}
		t.Logf("ingested %-34s %2d passages, %3d tokens, %5.1fs  (%s)", file, saved.Passages, saved.Tokens, time.Since(at).Seconds(), saved.Message())
	}
	t.Logf("all three ingested in %.1fs", time.Since(start).Seconds())

	questions := []struct {
		n        int
		asker    string
		question string
		file     string // the document that holds the answer; empty when nothing should match
	}{
		{1, "Delivery", "What do I do when a priority-one ticket breaches its SLA?", "delivery-and-escalation-sop.md"},
		{2, "Delivery", "When am I not allowed to deploy to production?", "delivery-and-escalation-sop.md"},
		{3, "Delivery", "What discount can I offer a client who commits to a twelve month retainer?", "pricing-and-proposal-playbook.md"},
		{4, "Delivery", "How many days of annual leave do I get?", "leave-and-benefits-policy.md"},
		{5, "Delivery", "What is the office Wi-Fi password?", ""},
		{6, "Sales & Marketing", "Who has to approve a discount above 10%?", "pricing-and-proposal-playbook.md"},

		// Overlap probes (logged, never asserted). Pair A: the Delivery Director is
		// named in both the SOP (P1 breach) and the playbook (larger discounts).
		// Pair B: the SOP's weekly meeting and the HR policy's team lead.
		{7, "Delivery", "Who is the Delivery Director and when do I involve them?", ""},
		{8, "Sales & Marketing", "Does a big discount need the Delivery Director's sign-off?", ""},
		{9, "Delivery", "What happens in the weekly meeting?", ""},
		{10, "Delivery", "Does my team lead have to agree to my leave?", ""},
	}

	type hit struct {
		file    string
		index   int
		heading string
		score   float64
	}
	for _, q := range questions {
		res, err := client.Embed(ctx, []string{q.question}, embed.Query)
		if err != nil {
			t.Fatalf("Q%d embed: %v", q.n, err)
		}
		rows, err := f.pool.Query(ctx, `
			SELECT d.source_filename, c.chunk_index, split_part(c.content, E'\n', 1),
			       1 - (c.embedding <=> $1::vector)
			FROM document_chunks c JOIN documents d ON d.id = c.document_id
			ORDER BY c.embedding <=> $1::vector`, embed.Vector(res.Vectors[0]))
		if err != nil {
			t.Fatal(err)
		}
		var hits []hit
		for rows.Next() {
			var h hit
			if err := rows.Scan(&h.file, &h.index, &h.heading, &h.score); err != nil {
				t.Fatal(err)
			}
			hits = append(hits, h)
		}
		rows.Close()

		// Best score among what the asker's team may read, and among what it may not.
		var perm, forb *hit
		for i := range hits {
			h := &hits[i]
			if holder[h.file] == q.asker {
				if perm == nil {
					perm = h
				}
			} else if forb == nil {
				forb = h
			}
		}

		// Where the answer actually lives: the chunk that contains the demo span.
		wantIdx := -1
		if q.file != "" {
			wantIdx = answerChunk(t, q.question, q.file)
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Q%d (%s asks) %q\n", q.n, q.asker, q.question)
		for i, h := range hits[:4] {
			mark := " "
			if h.file == q.file && h.index == wantIdx {
				mark = "*"
			}
			fmt.Fprintf(&b, "   %s %d. %.4f  %-8s %-26s [%s]\n", mark, i+1, h.score, shortFile(h.file), h.heading, holder[h.file])
		}
		fmt.Fprintf(&b, "   best permitted %.4f (%s)   best forbidden %.4f (%s)   margin forbidden-permitted %+.4f",
			perm.score, perm.heading, forb.score, forb.heading, forb.score-perm.score)
		if len(hits) > 1 {
			fmt.Fprintf(&b, "\n   gap between 1st and 2nd overall: %.4f", hits[0].score-hits[1].score)
		}
		t.Log("\n" + b.String())

		if q.file != "" && (hits[0].file != q.file || hits[0].index != wantIdx) {
			t.Errorf("Q%d: the top passage is %s #%d %q, not the one holding the answer (%s #%d)",
				q.n, hits[0].file, hits[0].index, hits[0].heading, q.file, wantIdx)
		}
	}
}

func shortFile(f string) string {
	switch {
	case strings.HasPrefix(f, "pricing"):
		return "playbook"
	case strings.HasPrefix(f, "delivery"):
		return "SOP"
	}
	return "HR"
}

// answerChunk returns the index of the chunk that holds the demo answer span.
func answerChunk(t *testing.T, question, file string) int {
	t.Helper()
	for _, s := range demoSpans {
		if s.question != question || s.file != file {
			continue
		}
		text, chunks := loadDemo(t, file)
		from, to := strings.Index(text, s.from), strings.Index(text, s.to)
		for _, c := range chunks {
			if from >= c.Start && to+len(s.to) <= c.End {
				return c.Index
			}
		}
	}
	t.Fatalf("no answer span for %q in %s", question, file)
	return -1
}
