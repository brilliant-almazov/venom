package runartifacts

import (
	"fmt"
	"strings"
)

// fixLink is the service to fix of a row that did not pass linked to its fix file; a row with none is an error, so no
// report names a red testcase without its fix.
func (l layout) fixLink(from string, r row) (string, error) {
	file, ok := l.fileOf[rowKey(r)]
	if !ok {
		return "", fmt.Errorf(noFixFile, r.testcase)
	}

	return link(cell(fixerOfRow(r).String()), from, file), nil
}

// fixLinks are the fixes of the rows that did not pass, each once, «—» when every one passed.
func (l layout) fixLinks(from string, rows []row) (string, error) {
	var out []string
	seen := map[string]bool{}

	for _, r := range rows {
		if passed(r) {
			continue
		}

		f, err := l.fixLink(from, r)
		if err != nil {
			return "", err
		}

		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}

	if len(out) == 0 {
		return noValue, nil
	}

	return strings.Join(out, "; "), nil
}
