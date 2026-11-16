package tempo

import (
	"fmt"
	"strings"
)

func Render(id, source string, body []byte) string {
	spans := readSpans(body)
	ids := map[string]bool{}
	for _, s := range spans {
		ids[s.SpanID] = true
	}
	children := map[string][]renderSpan{}
	for _, s := range spans {
		parent := s.ParentSpanID
		if !ids[parent] || parent == s.SpanID {
			parent = ""
		}
		children[parent] = append(children[parent], s)
	}
	var tree strings.Builder
	var order []renderSpan
	seenSQL := map[string]bool{}
	var walk func(parent, prefix string)
	walk = func(parent, prefix string) {
		kids := children[parent]
		for i, s := range kids {
			branch, next := "├─ ", prefix+"│  "
			if i == len(kids)-1 {
				branch, next = "└─ ", prefix+"   "
			}
			fmt.Fprintf(&tree, "%s%s%s · %s · %s "+wordMs+" · %s\n", prefix, branch, s.service, s.Name, ms(nanos(s.Start), nanos(s.End)), mark(s))
			order = append(order, s)
			under := next + "   "
			if s.SpanID != "" && len(children[s.SpanID]) > 0 {
				under = next + "│  "
			}
			for _, d := range details(s, seenSQL) {
				fmt.Fprintf(&tree, "%s%s\n", under, d)
			}
			if s.SpanID != "" {
				walk(s.SpanID, next)
			}
		}
	}
	walk("", "")
	return heading(id, source, children[""], order) + "```text\n" + tree.String() + "```\n"
}

func heading(id, source string, roots, order []renderSpan) string {
	root, took := "—", "—"
	if len(roots) > 0 {
		start, end := nanos(order[0].Start), nanos(order[0].End)
		for _, s := range order {
			start, end = min(start, nanos(s.Start)), max(end, nanos(s.End))
		}
		root, took = roots[0].service+" · "+roots[0].Name, ms(start, end)
	}
	bad, first := 0, ""
	for _, s := range order {
		if p := problem(s); p != "" {
			bad++
			if first == "" {
				first = wordFirst + s.service + " " + s.Name + " — " + p
			}
		}
	}
	if first == "" {
		first = wordNone
	}
	return fmt.Sprintf(wordHeading+"\n\n"+wordTable+"\n| -- | -- | -- | -- | -- | -- |\n"+
		"| `%s` | %s | %s | %d | %d | `%s` |\n\n"+wordSummary+"\n\n",
		id, id, root, took, len(order), bad, source, len(order)-bad, bad, first)
}
