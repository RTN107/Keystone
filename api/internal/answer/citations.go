package answer

import (
	"strconv"
	"strings"
)

// citationFilter sits between the model's token stream and the client. The model
// is asked to cite as [n], but a marker is only trusted if n names a passage that
// was actually supplied. The filter holds back a possible marker until it is
// complete, then either lets it through or drops it, so a made-up [9] never
// reaches the client and the streamed text equals the final text exactly. A list
// such as [1, 2] is rewritten as [1][2], so the client only ever sees single markers.
type citationFilter struct {
	passages int
	emit     func(string)

	pending string // a possible marker, starting at "["
	out     strings.Builder
	cited   []int // distinct valid numbers, first appearance order
	dropped int
}

const maxMarker = 16

func newCitationFilter(passages int, emit func(string)) *citationFilter {
	return &citationFilter{passages: passages, emit: emit}
}

// Write consumes one piece of streamed text.
func (f *citationFilter) Write(s string) {
	var run strings.Builder
	flushRun := func() {
		if run.Len() > 0 {
			f.send(run.String())
			run.Reset()
		}
	}
	for _, r := range s {
		if f.pending == "" {
			if r == '[' {
				f.pending = "["
			} else {
				run.WriteRune(r)
			}
			continue
		}
		switch {
		case r == ']':
			if text, ok := f.resolve(f.pending[1:]); ok {
				run.WriteString(text)
			} else {
				run.WriteString(f.pending + "]")
			}
			f.pending = ""
		case r == '[':
			run.WriteString(f.pending) // the earlier bracket was not a marker
			f.pending = "["
		case (r >= '0' && r <= '9') || r == ',' || r == ' ':
			f.pending += string(r)
			if len(f.pending) > maxMarker {
				run.WriteString(f.pending)
				f.pending = ""
			}
		default:
			run.WriteString(f.pending)
			f.pending = ""
			run.WriteRune(r)
		}
	}
	flushRun()
}

// Flush releases anything held back when the stream ends.
func (f *citationFilter) Flush() {
	if f.pending != "" {
		f.send(f.pending)
		f.pending = ""
	}
}

func (f *citationFilter) send(s string) {
	f.out.WriteString(s)
	if f.emit != nil {
		f.emit(s)
	}
}

// resolve turns the inside of a bracket into text to emit. ok is false when the
// bracket is not a citation at all, so it is left alone as ordinary text.
func (f *citationFilter) resolve(inner string) (string, bool) {
	parts := strings.Split(inner, ",")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err != nil || p == "" || len(p) > 3 {
			return "", false
		}
		nums = append(nums, n)
	}
	var kept strings.Builder
	for _, n := range nums {
		if n < 1 || n > f.passages {
			f.dropped++
			continue
		}
		kept.WriteString("[" + strconv.Itoa(n) + "]")
		if !contains(f.cited, n) {
			f.cited = append(f.cited, n)
		}
	}
	return kept.String(), true
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
