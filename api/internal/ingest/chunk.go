package ingest

import (
	"strings"
	"unicode"
)

// Chunk sizes are in characters (runes). Paragraphs are the unit: a paragraph of
// at least minChunk stands alone, shorter ones are joined with their neighbours
// until they reach it, and joining never takes a chunk past maxChunk. Small chunks
// keep the highlight a reader sees tight.
//
// A heading is never left stranded at the end of a chunk. If a heading and the
// paragraph under it would not fit in the current chunk, the heading starts the
// next one instead. The only overflow is a chunk that begins with a heading, which
// may exceed maxChunk by the heading's own length, up to maxHeadingChunk.
const (
	minChunk        = 350
	maxChunk        = 600
	maxHeadingChunk = maxChunk + 60
	tinyTail        = 120
)

// Chunk is one passage. Start and End are offsets into the normalized text in
// UTF-16 code units, so a browser can slice the text with them directly. Go's
// byte offsets would drift after the first curly quote or emoji.
type Chunk struct {
	Index int
	Start int
	End   int
	Text  string
}

// block is a run of lines or a piece of one, in rune offsets.
type block struct {
	start, end int // rune offsets, end exclusive
	heading    bool
}

// Split cuts normalized text into ordered, non-overlapping chunks.
func Split(text string) []Chunk {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	blocks := splitOversized(runes, paragraphs(runes))
	groups := pack(blocks)

	// u16[i] is the UTF-16 offset of rune i; the last entry is the total length.
	u16 := make([]int, len(runes)+1)
	for i, r := range runes {
		n := 1
		if r >= 0x10000 {
			n = 2
		}
		u16[i+1] = u16[i] + n
	}

	chunks := make([]Chunk, 0, len(groups))
	for i, g := range groups {
		chunks = append(chunks, Chunk{
			Index: i,
			Start: u16[g.start],
			End:   u16[g.end],
			Text:  string(runes[g.start:g.end]),
		})
	}
	return chunks
}

// paragraphs finds runs of non-blank lines. A single line starting with # is a
// heading, which never stands alone as a chunk.
func paragraphs(r []rune) []block {
	var out []block
	var (
		inPara     bool
		start, end int
		lines      int
	)
	flush := func() {
		if inPara {
			out = append(out, block{start: start, end: end, heading: lines == 1 && r[start] == '#'})
			inPara = false
		}
	}
	for i := 0; i < len(r); {
		j := i
		for j < len(r) && r[j] != '\n' {
			j++
		}
		if isBlank(r[i:j]) {
			flush()
		} else {
			if !inPara {
				inPara, start, lines = true, i, 0
			}
			lines++
			end = j
		}
		i = j + 1
	}
	flush()
	return out
}

func isBlank(line []rune) bool {
	for _, c := range line {
		if !unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

// splitOversized breaks any block longer than maxChunk at sentence or line ends,
// falling back to word and then character boundaries.
func splitOversized(r []rune, in []block) []block {
	var out []block
	for _, b := range in {
		if b.end-b.start <= maxChunk {
			out = append(out, b)
			continue
		}
		out = append(out, splitBlock(r, b)...)
	}
	return out
}

func splitBlock(r []rune, b block) []block {
	var out []block
	pos := b.start
	for pos < b.end {
		for pos < b.end && unicode.IsSpace(r[pos]) {
			pos++
		}
		if pos >= b.end {
			break
		}
		limit := min(pos+maxChunk, b.end)
		cut := limit
		if limit < b.end {
			cut = bestCut(r, pos, limit)
		}
		end := cut
		for end > pos && unicode.IsSpace(r[end-1]) {
			end--
		}
		out = append(out, block{start: pos, end: end})
		pos = cut
	}
	return out
}

// bestCut picks where to end a piece that must stop at or before limit: after the
// last sentence end or line break in the second half, else at the last space,
// else at limit itself.
func bestCut(r []rune, start, limit int) int {
	half := start + (limit-start)/2
	for i := limit; i > half; i-- {
		// i is a candidate cut: the piece is r[start:i]
		prev := r[i-1]
		if prev == '\n' {
			return i
		}
		if (prev == '.' || prev == '!' || prev == '?') && (i == len(r) || unicode.IsSpace(r[i])) {
			return i
		}
	}
	for i := limit; i > start+1; i-- {
		if unicode.IsSpace(r[i-1]) || (i < len(r) && unicode.IsSpace(r[i])) {
			return i
		}
	}
	return limit
}

// pack joins blocks into chunks. The span of a chunk runs from the start of its
// first block to the end of its last, so blank lines between blocks stay inside it
// and Text is always an exact slice of the document.
func pack(blocks []block) []block {
	var out []block
	for i := 0; i < len(blocks); {
		first := i
		cur := blocks[i]
		i++
		for i < len(blocks) {
			next := blocks[i]
			mustAbsorb := blocks[i-1].heading // never end a chunk on a heading
			if !mustAbsorb && cur.end-cur.start >= minChunk {
				break
			}
			limit := maxChunk
			if mustAbsorb && i-1 == first {
				limit = maxHeadingChunk // the chunk is only a heading so far
			}
			if next.heading {
				// Only take a heading if the paragraph under it fits too.
				follow := next.end
				if i+1 < len(blocks) {
					follow = blocks[i+1].end
				}
				if follow-cur.start > maxChunk {
					break
				}
			}
			if next.end-cur.start > limit {
				break
			}
			cur.end = next.end
			cur.heading = next.heading
			i++
		}
		out = append(out, cur)
	}

	// A short final chunk reads badly alone; fold it into the previous one if it fits.
	if n := len(out); n >= 2 {
		last, prev := out[n-1], out[n-2]
		if last.end-last.start < tinyTail && last.end-prev.start <= maxChunk {
			out[n-2].end = last.end
			out = out[:n-1]
		}
	}
	return out
}

// Preview returns the chunk text on one line, for logs and errors.
func (c Chunk) Preview() string {
	s := strings.Join(strings.Fields(c.Text), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "..."
	}
	return s
}
