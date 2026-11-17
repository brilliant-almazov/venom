package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// stepLines are the facts of a step under its line in a zoom: the executor, the call, the database and the SQL text
// verbatim line by line, the exchange, routing key and queue, every assertion, the trace id, the reason of a step that
// is not ok, then every step of the user executor it calls with the facts of its own, a level deeper.
func stepLines(prefix string, s venom.StepView) []string {
	var lines []string

	add := func(key, value string) {
		if value != "" {
			lines = append(lines, prefix+key+": "+value)
		}
	}

	add("executor", s.Type)
	add("call", strings.TrimSpace(s.Method+" "+s.URL))

	if s.Method == "" && s.URL == "" && s.Database == "" {
		add("call", s.Request)
	}

	add("database", s.Database)

	for _, q := range s.SQL {
		lines = append(lines, prefix+"sql:")

		for _, line := range strings.Split(q, "\n") {
			lines = append(lines, prefix+"  "+line)
		}
	}

	for _, q := range s.Queue {
		lines = append(lines, prefix+q)
	}

	for _, a := range s.Assertions {
		add("assert", a)
	}

	add("trace", s.TraceID)

	if s.Mark != venom.MarkPassed {
		add("reason", strings.Join(strings.Fields(s.Reason), " "))
	}

	for _, c := range s.Calls {
		lines = append(lines, prefix+fmt.Sprintf("→ %d %s", c.Number, c.Name))
		lines = append(lines, stepLines(prefix+"  ", c)...)
	}

	return lines
}
