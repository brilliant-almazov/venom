package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// writeProblems writes PROBLEMS.md: every testcase that did not pass — failed, skipped, interrupted, not run — by
// priority (problemsOf), with the index of its problem, its kind and criticality, its first failed step, the reason,
// the scenario line, the protocol, the scheme, the trace and the service to fix.
func (l layout) writeProblems(w *Writer) error {
	base := dirOf(l.problems())
	var b strings.Builder
	b.WriteString("# Problems\n\n")

	if w.runErr != "" {
		fmt.Fprintf(&b, "- run ended early: %s\n\n", one(w.runErr))
	}

	b.WriteString("| problem | kind | criticality | testcase | path | verdict | step | reason | scenario | protocol | scheme | trace | " +
		wordFixer + " |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- | -- |\n")
	n := 0

	for _, r := range problemsOf(l.rows) {
		fix, err := l.fixLink(base, r)
		if err != nil {
			return err
		}

		n++
		step, line := problemStep(r)
		protocol := noValue

		if written(r) {
			protocol = link(protocolLink, base, r.protocol)
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", problemID(r),
			cell(venom.KindText(levelOf(r.view.Kind))), cell(venom.CriticalityText(levelOf(r.view.Criticality))), cell(r.testcase), cell(l.pathOf(r)),
			verdictOf(r), step, one(r.view.Reason), scenarioLink(base, r, line), protocol, link(wordScheme, base, l.caseScheme(r)),
			traceCell(base, r), fix)
	}

	if n == 0 {
		b.WriteString("| no problems | | | | | | | | | | | | |\n")
	}

	return writeFile(l.problems(), []byte(b.String()))
}

// problemStep is the first failed step of the testcase and its line, else its first step not run, else the testcase.
func problemStep(r row) (string, int) {
	if s, ok := failedStep(r); ok {
		return fmt.Sprintf("%d %s", s.Number, one(s.Name)), s.Line
	}

	return noValue, r.view.Line
}

// failedStep is the first failed step of the testcase, else its first step not run.
func failedStep(r row) (venom.StepView, bool) {
	for _, want := range []string{venom.MarkFailed, venom.MarkNotRun} {
		for _, s := range r.view.Steps {
			if s.Mark == want {
				return s, true
			}
		}
	}

	return venom.StepView{}, false
}
