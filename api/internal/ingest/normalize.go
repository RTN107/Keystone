// Package ingest turns an uploaded file into stored, searchable passages:
// extract the text, normalize it, split it into chunks with exact offsets, embed
// them, and save everything in one transaction.
package ingest

import (
	"strings"
	"unicode"
)

// Normalize produces the text that is stored as documents.content. Chunk offsets
// index into this exact string, so it is computed once, before chunking, and
// never altered afterwards.
//
// It makes the text valid UTF-8, unifies line endings, removes control
// characters and trailing spaces, limits runs of blank lines to one, and trims
// the whole.
func Normalize(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = strings.TrimPrefix(s, "\uFEFF") // byte order mark
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == '\u00A0' || r == '\u2028' || r == '\u2029':
			b.WriteRune(' ')
		case unicode.IsControl(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}

	lines := strings.Split(b.String(), "\n")
	out := make([]string, 0, len(lines))
	blanks := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blanks++
			if blanks > 1 {
				continue
			}
		} else {
			blanks = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
