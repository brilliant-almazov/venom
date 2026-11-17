package runartifacts

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/ovh/venom"
)

// runner is the node every scenario starts from.
const runner = "venom"

// node is what the steps of the run touch — a host by its paths, a database by its tables, an executor by its calls —
// with the count of the steps on it.
type node struct {
	name, kind string
	c          count
	subs       []*node
	at         map[string]*node
	cases      map[string]bool
}

// edge is a hop in call order: from the node of a step to the node of the next step of the same testcase.
type edge struct {
	from, to string
	c        count
}

// sqlTable is a table an SQL text reads or writes: FROM, JOIN, INTO, UPDATE, TRUNCATE.
var sqlTable = regexp.MustCompile(`(?i)\b(?:from|join|into|update|truncate(?:\s+table)?)\s+((?:"[^"]+"|[a-z_][\w$]*)(?:\.(?:"[^"]+"|[a-z_][\w$]*))?)`)

// placeOf is the node of a step and the places on it: the host and the method with the path of an http call, the
// host and the rpc of a grpc call, the database and the tables of an sql step, else the executor and the request.
func placeOf(s venom.StepView) (string, []string) {
	switch {
	case s.Database != "":
		return "sql " + s.Database, tablesOf(s.SQL)

	case strings.Contains(s.URL, "://"):
		u, err := url.Parse(s.URL)
		if err != nil || u.Host == "" {
			return orNone(s.Type) + " " + s.URL, []string{s.Method}
		}

		return orNone(s.Type) + " " + u.Host, []string{strings.TrimSpace(s.Method + " " + orEmpty(u.Path, "/"))}

	case s.URL != "":
		return orNone(s.Type) + " " + strings.SplitN(s.URL, "/", 2)[0], []string{s.Method}

	default:
		return orNone(s.Type), []string{s.Request}
	}
}

// tablesOf are the tables of the SQL texts in the order met, «sql» when none is named; a name right before «(» is a
// function, not a table.
func tablesOf(texts []string) []string {
	var tables []string
	seen := map[string]bool{}

	for _, text := range texts {
		for _, m := range sqlTable.FindAllStringSubmatchIndex(text, -1) {
			name := strings.ToLower(text[m[2]:m[3]])

			if strings.HasPrefix(text[m[3]:], "(") || seen[name] {
				continue
			}

			seen[name] = true
			tables = append(tables, "table "+name)
		}
	}

	if len(tables) == 0 {
		return []string{"sql"}
	}

	return tables
}

// add counts the step mark on c.
func (c *count) add(mark string) {
	c.all++

	switch mark {
	case venom.MarkPassed:
		c.ok++
		c.passed++

	case venom.MarkFailed:
		c.notOK++
		c.failed++

	default:
		c.partial++
		c.other++
	}
}
