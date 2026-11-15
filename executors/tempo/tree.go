package tempo

import (
	"fmt"
	"regexp"
)

// tree is the spans of a trace in start order with their lines (SpanLines) and the index of each span by its id.
type tree struct {
	spans []renderSpan
	lines []string
	byID  map[string]int
}

func readTree(body []byte) tree {
	t := tree{spans: readSpans(body), lines: SpanLines(body), byID: map[string]int{}}
	for i, s := range t.spans {
		if s.SpanID != "" {
			t.byID[s.SpanID] = i
		}
	}
	return t
}

// parent is the index of the parent of span i, -1 when its parent is not in the trace.
func (t tree) parent(i int) int {
	if i < 0 {
		return -1
	}
	p, ok := t.byID[t.spans[i].ParentSpanID]
	if !ok || p == i {
		return -1
	}
	return p
}

// under says whether span i is a descendant of span root.
func (t tree) under(i, root int) bool {
	for p, n := t.parent(i), 0; p >= 0 && n < len(t.spans); p, n = t.parent(p), n+1 {
		if p == root {
			return true
		}
	}
	return false
}

// lineOf is the line of span i, empty for -1.
func (t tree) lineOf(i int) string {
	if i < 0 {
		return ""
	}
	return t.lines[i]
}

// compile is a regexp of an input named name, nil when expr is empty.
func compile(name, expr string) (*regexp.Regexp, error) {
	if expr == "" {
		return nil, nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("tempo: %s %q: %w", name, expr, err)
	}
	return re, nil
}
