package tempo

import "sort"

type renderEvent struct {
	Name       string       `json:"name"`
	Attributes []renderAttr `json:"attributes"`
}

func details(s renderSpan, seenSQL map[string]bool) []string {
	var lines []string
	if p := problem(s); p != "" {
		lines = append(lines, wordProblem+p)
	}
	query, hasQuery := "", false
	for _, a := range sortedByKey(s.Attributes) {
		if a.Key == "db.query.text" {
			query, hasQuery = a.Value.pyText(), true
			continue
		}
		lines = append(lines, a.Key+": "+a.Value.pyText())
	}
	for _, e := range s.Events {
		lines = append(lines, wordEvent+e.Name)
		for _, a := range sortedByKey(e.Attributes) {
			lines = append(lines, "   "+a.Key+": "+a.Value.pyText())
		}
	}
	if hasQuery && seenSQL[query] {
		lines = append(lines, wordSQL+wordSQLSame)
	} else if hasQuery {
		seenSQL[query] = true
		for i, l := range sqlLines(query) {
			if i == 0 {
				lines = append(lines, wordSQL+l)
			} else {
				lines = append(lines, wordSQLAfter+l)
			}
		}
	}
	return lines
}

func sortedByKey(attrs []renderAttr) []renderAttr {
	out := append([]renderAttr{}, attrs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
