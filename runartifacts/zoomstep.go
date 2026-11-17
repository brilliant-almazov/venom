package runartifacts

import (
	"strings"

	"github.com/ovh/venom"
)

// stepLines are the facts of a step under its line in a zoom: the executor, the call, the database and the SQL text
// verbatim line by line, every assertion, the trace id and the reason of a step that is not ok.
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

	for _, a := range s.Assertions {
		add("assert", a)
	}

	add("trace", s.TraceID)

	if s.Mark != venom.MarkPassed {
		add("reason", strings.Join(strings.Fields(s.Reason), " "))
	}

	return lines
}
