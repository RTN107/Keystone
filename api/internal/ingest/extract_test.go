package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// buildPDF writes a minimal valid one-page PDF with one line of text per entry.
func buildPDF(lines []string) []byte {
	var buf bytes.Buffer
	var offsets []int
	obj := func(s string) { offsets = append(offsets, buf.Len()); buf.WriteString(s) }

	var content strings.Builder
	if len(lines) > 0 {
		content.WriteString("BT /F1 12 Tf 72 720 Td 14 TL\n")
		for i, l := range lines {
			l = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(l)
			if i > 0 {
				content.WriteString("T*\n")
			}
			fmt.Fprintf(&content, "(%s) Tj\n", l)
		}
		content.WriteString("ET\n")
	}

	buf.WriteString("%PDF-1.4\n")
	obj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	obj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	obj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")
	obj(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", content.Len(), content.String()))
	obj("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return buf.Bytes()
}

func TestNormalize(t *testing.T) {
	bom := string(rune(0xFEFF))
	cases := []struct{ name, in, want string }{
		{"line endings", "a\r\nb\rc", "a\nb\nc"},
		{"bom", bom + "hello", "hello"},
		{"control characters", "a\x00b\x07c", "abc"},
		{"trailing spaces", "line one   \nline two\t\n", "line one\nline two"},
		{"blank runs", "a\n\n\n\n\nb", "a\n\nb"},
		{"outer whitespace", "\n\n  hello  \n\n", "hello"},
		{"invalid utf8 run becomes one replacement", "ok\xff\xfeok", "ok" + string(rune(0xFFFD)) + "ok"},
		{"nbsp", "a b", "a b"},
		{"curly quotes kept", "it’s", "it’s"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	// Normalizing twice must change nothing, or stored offsets could drift.
	once := Normalize("  a\r\n\r\n\r\nb  \x00   c\n")
	if Normalize(once) != once {
		t.Error("Normalize is not idempotent")
	}
}

func TestExtractTextAndMarkdown(t *testing.T) {
	got, err := Extract("policy.TXT", []byte("\xef\xbb\xbfFirst line\r\n\r\nSecond line  \r\n"))
	if err != nil || got != "First line\n\nSecond line" {
		t.Errorf("txt: %q %v", got, err)
	}
	got, err = Extract("notes.md", []byte("# Heading\n\nBody text."))
	if err != nil || got != "# Heading\n\nBody text." {
		t.Errorf("md: %q %v", got, err)
	}
}

func TestExtractRefusals(t *testing.T) {
	if _, err := Extract("slides.pptx", []byte("x")); !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("unsupported type: %v", err)
	}
	if _, err := Extract("empty.txt", []byte("  \n\n ")); !errors.Is(err, ErrEmptyDocument) {
		t.Errorf("empty text: %v", err)
	}
	if _, err := Extract("notpdf.pdf", []byte("this is plain text")); !errors.Is(err, ErrUnreadablePDF) {
		t.Errorf("fake pdf: %v", err)
	}
}

func TestExtractPDF(t *testing.T) {
	pdfBytes := buildPDF([]string{"Escalation procedure", "Page the delivery lead within 15 minutes (P1 only)."})
	got, err := Extract("sop.pdf", pdfBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Escalation procedure", "Page the delivery lead within 15 minutes (P1 only)."} {
		if !strings.Contains(got, want) {
			t.Errorf("PDF text missing %q; got %q", want, got)
		}
	}
}

func TestExtractPDFWithoutTextSaysSo(t *testing.T) {
	if _, err := Extract("scan.pdf", buildPDF(nil)); !errors.Is(err, ErrNoPDFText) {
		t.Errorf("want ErrNoPDFText, got %v", err)
	}
}

func TestMalformedPDFsNeverPanic(t *testing.T) {
	good := buildPDF([]string{"Some text here"})
	inputs := [][]byte{
		good[:len(good)/2],
		append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0xff, 0x00, '<', '<'}, 400)...),
		[]byte("%PDF-"),
		bytes.ReplaceAll(good, []byte("xref"), []byte("xxxx")),
	}
	for i, in := range inputs {
		if _, err := Extract("broken.pdf", in); err == nil {
			t.Logf("input %d parsed without error; acceptable", i)
		}
	}
}
