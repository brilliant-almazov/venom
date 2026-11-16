package tempo

import (
	"sort"
	"strings"
)

// prefixed are "<key>: <value>" of the attributes of a span whose key starts with prefix, sorted by key.
func prefixed(s renderSpan, prefix string) []string {
	var lines []string
	for _, a := range s.Attributes {
		if strings.HasPrefix(a.Key, prefix) {
			lines = append(lines, a.Key+": "+a.text())
		}
	}
	sort.Strings(lines)
	return lines
}

// codeSpan is v as one Markdown code span, its line breaks kept as spaces, fenced by more backticks than any run in v.
func codeSpan(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	fence, run := "`", 0
	for _, r := range v {
		if r == '`' {
			run++
			if run >= len(fence) {
				fence += "`"
			}
		} else {
			run = 0
		}
	}
	if strings.HasPrefix(v, "`") || strings.HasSuffix(v, "`") {
		v = " " + v + " "
	}
	return fence + v + fence
}
