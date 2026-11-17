package runartifacts

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ovh/venom"
)

// scenarioLink links the line of the testsuite file: the text `<file>:<line>`, the target `<file>#L<line>`.
func scenarioLink(from string, r row, line int) string {
	text := filepath.ToSlash(r.suiteFile)

	if line <= 0 {
		return link(text, from, r.absFile)
	}

	return fmt.Sprintf("[%s:%d](%s#L%d)", text, line, relTarget(from, r.absFile), line)
}

// traceRef is a trace id with a link to its span tree, «trace not saved» when the run has none.
func traceRef(from string, r row, id string) string {
	if tree, ok := r.trees[id]; ok {
		return "`" + id + "` " + link("span tree", from, tree)
	}

	return "`" + id + "` " + noTree
}

// traceIDs are the trace ids of the row, the call's first, then the steps' in step order.
func traceIDs(r row) []string {
	var ids []string
	seen := map[string]bool{}

	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	add(r.view.TraceID)

	for _, s := range r.view.Steps {
		add(s.TraceID)
	}

	return ids
}

// verdictOf is the mark of the testcase with its answer code, or the state of a testcase that did not end.
func verdictOf(r row) string {
	if r.state == stateDone {
		return verdict(r.view.Mark, r.view.Code)
	}

	return venom.MarkNotRun + " " + r.state
}

// passed: the testcase ended and passed.
func passed(r row) bool {
	return r.state == stateDone && r.view.Mark == venom.MarkPassed
}

// failed: the testcase ended and failed.
func failed(r row) bool {
	return r.state == stateDone && r.view.Mark == venom.MarkFailed
}

// links is the «## Links» section of a protocol: the scenario and its call by line, every trace id, the schemes of the
// case, its path and the system, the index, the report and the problems.
func (w *Writer) links(r *row) string {
	l := w.layout()
	from := filepath.Dir(r.protocol)
	var b strings.Builder
	b.WriteString(headLinks + "\n\n")
	fmt.Fprintf(&b, "- scenario: %s\n", scenarioLink(from, *r, r.view.Line))

	if n := r.view.CallStep; n > 0 {
		fmt.Fprintf(&b, "- call: step %d %s\n", n, scenarioLink(from, *r, stepLineOf(r.view, n)))
	}

	ids := traceIDs(*r)

	for _, id := range ids {
		fmt.Fprintf(&b, "- trace id: %s\n", traceRef(from, *r, id))
	}

	if len(ids) == 0 {
		b.WriteString("- trace id: " + noValue + "\n")
	}

	fmt.Fprintf(&b, "- scheme of the case: %s\n", link(r.testcase, from, l.caseScheme(*r)))
	fmt.Fprintf(&b, "- scheme of the path: %s\n", link(l.pathOf(*r), from, l.pathScheme(l.pathOf(*r))))
	fmt.Fprintf(&b, "- scheme of the system: %s\n", link(SystemFile, from, l.systemScheme()))
	fmt.Fprintf(&b, "- run: %s · %s · %s\n", link(IndexFile, from, l.index()), link(ReportFile, from, l.report()),
		link(ProblemsFile, from, l.problems()))

	return b.String()
}

// stepLineOf is the line of the 1-based step n of the view.
func stepLineOf(v venom.ProtocolView, n int) int {
	for _, s := range v.Steps {
		if s.Number == n {
			return s.Line
		}
	}

	return v.Line
}

// withLinks puts the links section after the title and the file line of a protocol.
func withLinks(text, links string) string {
	at := strings.Index(text, "\n\n## ")

	if at < 0 {
		return text + "\n" + links
	}

	return text[:at+2] + links + "\n" + text[at+2:]
}

const headLinks = "## Links"
