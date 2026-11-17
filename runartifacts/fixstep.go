package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// writeExpectedGot writes what the testcase expected and what came: the failed step and its scenario line, the call,
// the expected behaviour of the scenario, the code, the reason and the body.
func writeExpectedGot(b *strings.Builder, base string, r row) {
	fmt.Fprintf(b, "\n### %s\n\n", r.testcase)
	step, ok := failedStep(r)

	if !ok {
		fmt.Fprintf(b, "- verdict: %s\n- reason: %s\n- scenario: %s\n", verdictOf(r), orNone(one(r.view.Reason)),
			scenarioLink(base, r, r.view.Line))
		return
	}

	fmt.Fprintf(b, "- step: %d %s — %s\n", step.Number, one(step.Name), scenarioLink(base, r, step.Line))
	fmt.Fprintf(b, "- call: %s\n", orNone(step.Request))
	expected := expectedLines(r, false)

	if len(expected) == 0 {
		expected = []string{noExpected}
	}

	for _, e := range expected {
		fmt.Fprintf(b, "- expected: %s\n", e)
	}

	fmt.Fprintf(b, "- got: %s %s · %s\n", step.Mark, orNone(step.Code), orNone(one(step.Reason)))
	b.WriteString(venom.ProtocolBodyBlock("body", step.Body))
}

// expectedLines are the info lines of the testcase that are norms when norm, else the expected behaviour: happens,
// expect and right.
func expectedLines(r row, norm bool) []string {
	var out []string

	for _, e := range r.view.Expected {
		if strings.HasPrefix(e, normKey) == norm {
			out = append(out, e)
		}
	}

	return out
}
