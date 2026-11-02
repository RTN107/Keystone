package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// demoSpans are the answers the demo depends on. Each span runs from the first
// words to the last words of the text that answers the question. If an answer
// straddles a chunk boundary, retrieval can return half of it, so the fix is to
// reword the document, never to loosen the chunking.
var demoSpans = []struct {
	question, file, from, to string
}{
	{
		"What do I do when a priority-one ticket breaches its SLA?",
		"delivery-and-escalation-sop.md",
		"A priority-one (P1) ticket breaches its SLA",
		"inside 15 minutes of the bridge opening.",
	},
	{
		"When am I not allowed to deploy to production?",
		"delivery-and-escalation-sop.md",
		"No production changes are made on Fridays",
		"before they are deployed.",
	},
	{
		"What discount can I offer a client who commits to a twelve month retainer?",
		"pricing-and-proposal-playbook.md",
		"Clients who commit to a twelve month retainer",
		"cloud costs that we pass through at cost.",
	},
	{
		"Who has to approve a discount above 10%?",
		"pricing-and-proposal-playbook.md",
		"Any discount above 10% needs written approval",
		"We never offer a discount in a first meeting.",
	},
	{
		"How many days of annual leave do I get?",
		"leave-and-benefits-policy.md",
		"Every permanent employee earns 18 days",
		"after which they lapse.",
	},
}

func loadDemo(t *testing.T, file string) (string, []Chunk) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "demo-docs", file))
	if err != nil {
		t.Fatal(err)
	}
	text, err := Extract(file, data)
	if err != nil {
		t.Fatal(err)
	}
	return text, Split(text)
}

func TestDemoAnswersEachSitInOneChunk(t *testing.T) {
	for _, s := range demoSpans {
		text, chunks := loadDemo(t, s.file)
		from, to := strings.Index(text, s.from), strings.Index(text, s.to)
		if from < 0 || to < 0 {
			t.Errorf("%q: span text not found in %s (from=%d to=%d)", s.question, s.file, from, to)
			continue
		}
		end := to + len(s.to)

		// chunks hold UTF-16 offsets; the demo text is ASCII, so byte offsets match.
		var holders []int
		for _, c := range chunks {
			if from >= c.Start && end <= c.End {
				holders = append(holders, c.Index)
			}
		}
		if len(holders) != 1 {
			t.Errorf("%q: the answer is not inside a single chunk of %s", s.question, s.file)
			for _, c := range chunks {
				t.Logf("  chunk %d [%d,%d): %s", c.Index, c.Start, c.End, c.Preview())
			}
		}
	}
}

// TestDemoSectionsAreEachOneChunk pins the document design: every heading begins
// its own chunk, so a citation highlights one section and nothing around it.
func TestDemoSectionsAreEachOneChunk(t *testing.T) {
	for _, f := range []string{"pricing-and-proposal-playbook.md", "delivery-and-escalation-sop.md", "leave-and-benefits-policy.md"} {
		text, chunks := loadDemo(t, f)
		headings := 0
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "#") {
				headings++
			}
		}
		if len(chunks) != headings {
			t.Errorf("%s has %d headings but %d chunks", f, headings, len(chunks))
			continue
		}
		for _, c := range chunks {
			if !strings.HasPrefix(c.Text, "#") {
				t.Errorf("%s chunk %d does not begin at a heading: %q", f, c.Index, c.Preview())
			}
			if n := len([]rune(c.Text)); n < minChunk || n > maxChunk {
				t.Errorf("%s chunk %d is %d characters, outside %d to %d", f, c.Index, n, minChunk, maxChunk)
			}
		}
	}
}

func TestDemoDocumentsAreNeutralAndWellFormed(t *testing.T) {
	all, _ := filepath.Glob(filepath.Join("..", "..", "..", "demo-docs", "*.md"))
	var files []string // the three documents, not the folder's own README
	for _, f := range all {
		if filepath.Base(f) != "README.md" {
			files = append(files, f)
		}
	}
	if len(files) != 3 {
		t.Fatalf("want 3 demo documents, found %d", len(files))
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		if strings.ContainsRune(s, rune(0x2014)) || strings.ContainsRune(s, rune(0x2013)) {
			t.Errorf("%s contains an em or en dash; the demo documents avoid them", filepath.Base(f))
		}
		_, chunks := loadDemo(t, filepath.Base(f))
		for _, c := range chunks {
			if n := len([]rune(c.Text)); n < 100 {
				t.Errorf("%s chunk %d is only %d characters, too thin to retrieve well: %q", filepath.Base(f), c.Index, n, c.Preview())
			}
		}
	}
}

// TestPrintDemoChunks is a review aid: go test -run PrintDemoChunks -v ./internal/ingest
func TestPrintDemoChunks(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("verbose only")
	}
	for _, f := range []string{"pricing-and-proposal-playbook.md", "delivery-and-escalation-sop.md", "leave-and-benefits-policy.md"} {
		_, chunks := loadDemo(t, f)
		t.Logf("== %s: %d chunks", f, len(chunks))
		for _, c := range chunks {
			t.Logf("  %d [%4d,%4d) %3d chars  %s", c.Index, c.Start, c.End, len([]rune(c.Text)), c.Preview())
		}
	}
}
