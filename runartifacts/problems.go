package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// writeProblems writes PROBLEMS.md: every testcase that did not pass — failed, skipped, interrupted, not run — with its
// first failed step, the reason, the scenario line, the protocol, the scheme and the trace.
func (l layout) writeProblems(w *Writer) error {
	base := dirOf(l.problems())
	var b strings.Builder
	b.WriteString("# Problems\n\n")

	if w.runErr != "" {
		fmt.Fprintf(&b, "- run ended early: %s\n\n", one(w.runErr))
	}

	b.WriteString("| testcase | path | verdict | step | reason | scenario | protocol | scheme | trace |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- | -- |\n")
	n := 0

	for _, r := range l.rows {
		if passed(r) {
			continue
		}

		n++
		step, line := problemStep(r)
		protocol := noValue

		if written(r) {
			protocol = link(protocolLink, base, r.protocol)
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", cell(r.testcase), cell(l.pathOf(r)), verdictOf(r),
			step, one(r.view.Reason), scenarioLink(base, r, line), protocol, link(wordScheme, base, l.caseScheme(r)),
			traceCell(base, r))
	}

	if n == 0 {
		b.WriteString("| no problems | | | | | | | | |\n")
	}

	return writeFile(l.problems(), []byte(b.String()))
}

// problemStep is the first failed step of the testcase and its line, else its first step not run, else the testcase.
func problemStep(r row) (string, int) {
	for _, want := range []string{venom.MarkFailed, venom.MarkNotRun} {
		for _, s := range r.view.Steps {
			if s.Mark == want {
				return fmt.Sprintf("%d %s", s.Number, one(s.Name)), s.Line
			}
		}
	}

	return noValue, r.view.Line
}
