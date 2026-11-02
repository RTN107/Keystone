package ingest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"
)

// slice16 cuts text by UTF-16 offsets, which is exactly what the browser does.
func slice16(text string, start, end int) string {
	u := utf16.Encode([]rune(text))
	return string(utf16.Decode(u[start:end]))
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// checkInvariants is what every chunking, however odd the input, must satisfy.
func checkInvariants(t *testing.T, text string, chunks []Chunk) {
	t.Helper()
	total := len(utf16.Encode([]rune(text)))
	prevEnd := 0
	var joined strings.Builder
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("chunk %d has index %d", i, c.Index)
		}
		if got := slice16(text, c.Start, c.End); got != c.Text {
			t.Fatalf("chunk %d: offsets [%d,%d) give %q but Text is %q", i, c.Start, c.End, got, c.Text)
		}
		if c.Start < prevEnd {
			t.Errorf("chunk %d overlaps the previous one (%d < %d)", i, c.Start, prevEnd)
		}
		if c.End > total || c.Start >= c.End {
			t.Errorf("chunk %d has a bad range [%d,%d) of %d", i, c.Start, c.End, total)
		}
		if strings.TrimSpace(c.Text) == "" {
			t.Errorf("chunk %d is blank", i)
		}
		if n := len([]rune(c.Text)); n > maxHeadingChunk {
			t.Errorf("chunk %d is %d characters, over the %d ceiling", i, n, maxHeadingChunk)
		}
		prevEnd = c.End
		joined.WriteString(c.Text)
	}
	if stripSpace(joined.String()) != stripSpace(text) {
		t.Errorf("chunks do not cover the whole document")
	}
}

func TestOffsetsAreUTF16NotBytesNorRunes(t *testing.T) {
	// Curly quotes are 3 bytes but 1 unit; the emoji is 4 bytes, 1 rune, 2 units.
	text := "# Notes\n\n" +
		strings.Repeat("It’s “fine” to ship on Friday \U0001F680 if the checks pass. ", 8) + "\n\n" +
		"After the rocket \U0001F680 comes the second paragraph, which must still be located exactly. " +
		"It has enough words to be its own chunk once the first one is full of text and then some more."
	chunks := Split(text)
	if len(chunks) < 2 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	checkInvariants(t, text, chunks)

	// The second chunk starts after the emoji-bearing first one. If offsets were
	// runes or bytes, the browser slice would not equal Text; checkInvariants proved
	// it equals. Also prove the rune offset differs, so the test can tell them apart.
	last := chunks[len(chunks)-1]
	runeStart := len([]rune(text[:strings.Index(text, last.Text)]))
	if runeStart == last.Start {
		t.Errorf("test text should contain astral characters so UTF-16 and rune offsets differ")
	}
}

func TestParagraphsStandAloneAndShortOnesJoinTheNext(t *testing.T) {
	long := strings.Repeat("This paragraph is long enough to stand on its own as a chunk. ", 7) // ~430 chars
	text := "# Title\n\n" + long + "\n\nShort one.\n\nAnother short.\n\n" + long
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) != 2 {
		for _, c := range chunks {
			t.Logf("chunk %d: %q", c.Index, c.Preview())
		}
		t.Fatalf("want 2 chunks (heading+long, shorts+long), got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0].Text, "# Title") {
		t.Error("a heading must travel with the paragraph after it")
	}
	if strings.Contains(chunks[0].Text, "Short one.") {
		t.Error("a paragraph of 350 characters or more must not absorb its neighbours")
	}
	if !strings.HasPrefix(chunks[1].Text, "Short one.") {
		t.Error("short paragraphs should be joined together and onto the next one")
	}
}

func TestShortParagraphsBetweenLongOnesDoNotMergeBeyondTheCeiling(t *testing.T) {
	long := strings.Repeat("This paragraph is long enough to stand on its own as a chunk. ", 9) + "Plus a closing remark about it." // ~590, so a short note cannot join it
	text := long + "\n\nA short note.\n\n" + long
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
}

func TestHeadingIsNeverStrandedAtTheEndOfAChunk(t *testing.T) {
	long := strings.Repeat("A sentence that adds some bulk to the paragraph here. ", 9) // ~490
	text := long + "\n\n## Next section\n\n" + long
	for _, c := range Split(text) {
		lines := strings.Split(strings.TrimSpace(c.Text), "\n")
		if strings.HasPrefix(lines[len(lines)-1], "#") {
			t.Errorf("chunk %d ends with a heading: %q", c.Index, c.Preview())
		}
	}
}

func TestLongParagraphSplitsAtSentencesWithoutCuttingWords(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "Sentence number %d explains one more small detail about the process. ", i)
	}
	text := strings.TrimSpace(b.String())
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) < 4 {
		t.Fatalf("expected the long paragraph to split, got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if !strings.HasSuffix(c.Text, ".") {
			t.Errorf("chunk %d does not end at a sentence boundary: %q", c.Index, c.Preview())
		}
		if !strings.HasPrefix(c.Text, "Sentence") {
			t.Errorf("chunk %d does not start at a sentence boundary: %q", c.Index, c.Preview())
		}
	}
}

func TestUnbrokenTextIsStillCut(t *testing.T) {
	text := strings.Repeat("x", 1700)
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) < 3 {
		t.Errorf("want the run cut into pieces, got %d", len(chunks))
	}
}

func TestOneLinePerBlockListsPackTogether(t *testing.T) {
	text := "- first rule about change freezes\n- second rule about rollbacks\n- third rule about approvals"
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) != 1 {
		t.Errorf("a short list is one chunk, got %d", len(chunks))
	}
}

func TestEmptyAndTinyDocuments(t *testing.T) {
	if got := Split(""); got != nil {
		t.Errorf("empty text should give no chunks, got %v", got)
	}
	c := Split("Just one line.")
	if len(c) != 1 || c[0].Start != 0 || c[0].End != 14 {
		t.Errorf("tiny document: %+v", c)
	}
}

// TestInvariantsHoldOnRandomText throws awkward text at the chunker: mixed
// scripts, astral characters, headings, blank lines, very long and very short
// runs. Every output must satisfy the invariants.
func TestInvariantsHoldOnRandomText(t *testing.T) {
	words := []string{"the", "policy", "applies", "to", "every", "ticket", "“quoted”", "it’s", "\U0001F680", "café", "日本語", "done.", "Really?", "No!", "x", "approval", "within", "15", "minutes,"}
	rng := rand.New(rand.NewSource(7))
	for iter := 0; iter < 300; iter++ {
		var b strings.Builder
		for p := 0; p < 1+rng.Intn(8); p++ {
			if rng.Intn(4) == 0 {
				b.WriteString("## " + words[rng.Intn(len(words))] + " " + words[rng.Intn(len(words))] + "\n\n")
			}
			n := 1 + rng.Intn(140)
			for w := 0; w < n; w++ {
				b.WriteString(words[rng.Intn(len(words))])
				switch rng.Intn(12) {
				case 0:
					b.WriteString("\n")
				default:
					b.WriteString(" ")
				}
			}
			b.WriteString("\n\n")
		}
		text := Normalize(b.String())
		if text == "" {
			continue
		}
		checkInvariants(t, text, Split(text))
	}
}

// headingCase builds: a paragraph just under the minimum, a heading that lifts the
// chunk over it, then a paragraph of the given length.
func headingCase(secondParaLen int) (text string, a string) {
	a = strings.TrimSpace(strings.Repeat("Alpha paragraph text that is just short of the minimum size. ", 5) + "Closing line to pad this out a bit.")
	b := strings.TrimSpace(strings.Repeat("Beta paragraph text that follows the heading in the document. ", 10))
	r := []rune(b)
	b = strings.TrimSpace(string(r[:secondParaLen]))
	return a + "\n\n## Next section\n\n" + b, a
}

func lastLineIsHeading(c Chunk) bool {
	lines := strings.Split(strings.TrimSpace(c.Text), "\n")
	return strings.HasPrefix(lines[len(lines)-1], "#")
}

// The heading lifts the first chunk past the minimum. Without the absorb rule the
// chunk would stop right after the heading. Here the next paragraph fits, so the
// heading and its paragraph must stay together in the same chunk.
func TestHeadingThatFitsWithItsParagraphIsAbsorbed(t *testing.T) {
	text, a := headingCase(200)
	if n := len([]rune(a)); n >= minChunk || n+2+len("## Next section") < minChunk {
		t.Fatalf("test setup: paragraph is %d runes", n)
	}
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) != 1 {
		t.Fatalf("want the heading and its paragraph kept with the first chunk, got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if lastLineIsHeading(c) {
			t.Errorf("chunk %d ends with a heading", c.Index)
		}
	}
}

// Here the paragraph under the heading would push the chunk past the ceiling, so
// the heading moves to the next chunk and the first one stays within the limit.
func TestHeadingThatDoesNotFitStartsTheNextChunk(t *testing.T) {
	text, _ := headingCase(330)
	chunks := Split(text)
	checkInvariants(t, text, chunks)
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[1].Text, "## Next section") {
		t.Errorf("the heading should open the second chunk, got %q", chunks[1].Preview())
	}
	for _, c := range chunks {
		if len([]rune(c.Text)) > maxChunk {
			t.Errorf("chunk %d is %d characters, over %d", c.Index, len([]rune(c.Text)), maxChunk)
		}
		if lastLineIsHeading(c) {
			t.Errorf("chunk %d ends with a heading", c.Index)
		}
	}
}
