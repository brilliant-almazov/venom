package runartifacts

import (
	"fmt"
	"strings"
)

// writeCase writes schemes/<path>/<case>.md: the transitions of one case drawn, ok / not-ok on each with its reason,
// then the table of the transitions and how the case works, with the links to the path, the system, the protocol, the
// scenario lines and the traces.
func (l layout) writeCase(r row) error {
	file := l.caseScheme(r)
	base := dirOf(file)
	name := l.pathOf(r)
	var b strings.Builder
	fmt.Fprintf(&b, "# Scheme of the case %s\n\n", r.testcase)
	fmt.Fprintf(&b, "- verdict: %s\n- outcome: %s\n- transitions share: %s\n- path: %s · system: %s\n- scenario: %s\n",
		verdictOf(r), r.outcome, stepShare(r),
		link(name, base, l.pathScheme(name)), link(SystemFile, base, l.systemScheme()), scenarioLink(base, r, r.view.Line))

	protocol := noValue

	if written(r) {
		protocol = link(protocolLink, base, r.protocol)
	}

	fmt.Fprintf(&b, "- protocol: %s\n", protocol)

	for _, id := range traceIDs(r) {
		fmt.Fprintf(&b, "- trace id: %s\n", traceRef(base, r, id))
	}

	b.WriteString("\n```text\n" + caseNode(r) + "\n")
	drawSteps(&b, "", r.view.Steps)
	b.WriteString("```\n\n## Transitions\n\n")
	b.WriteString("| step | name | request | ok / not-ok | code | reason | scenario | trace |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- |\n")

	for _, s := range r.view.Steps {
		trace := noValue

		if s.TraceID != "" {
			trace = traceRef(base, r, s.TraceID)
		}

		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n", s.Number, one(s.Name), codeCell(s.Request), s.Mark,
			orNone(s.Code), orNone(one(s.Reason)), scenarioLink(base, r, s.Line), trace)
	}

	b.WriteString("\n## How it works\n\n")
	fmt.Fprintf(&b, "- request: %s", codeCell(r.view.Request))

	if r.view.CallStep > 0 {
		fmt.Fprintf(&b, " — step %d, %s", r.view.CallStep, scenarioLink(base, r, stepLineOf(r.view, r.view.CallStep)))
	}

	fmt.Fprintf(&b, "\n- expected: %s\n- transitions: %s\n", one(expectedOf(r)), transitionsOf(r))

	if r.view.Reason != "" {
		fmt.Fprintf(&b, "- reason: %s\n", one(r.view.Reason))
	}

	return writeFile(file, []byte(b.String()))
}

func orNone(s string) string {
	if s == "" {
		return noValue
	}

	return s
}
