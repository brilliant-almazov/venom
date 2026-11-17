package runartifacts

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
)

// zoomSuiteText is 03-suites/<testsuite>.md: the testsuite with its bar and history, then every testcase with every
// step — executor, method and url, the database and the SQL text verbatim, the assertions, the outcome, the trace id —
// and the links to the scenario line, the protocol and the scheme of the case.
func (l layout) zoomSuiteText(rows []row, past []pastRun, hist []map[string]count) string {
	file := l.zoomSuite(rows[0])
	base := dirOf(file)
	c := countOf(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# Testsuite %s\n\n", rows[0].suite)
	fmt.Fprintf(&b, "- file: %s\n- testcases: %s\n- zoom: %s · %s · %s\n\n", scenarioLink(base, rows[0], 0), c,
		link(l.pathOf(rows[0]), base, l.zoomPath(l.pathOf(rows[0]))), link(ZoomRunFile, base, l.zoomRun()),
		link(IndexFile, base, l.zoomIndex()))
	fence(&b, []string{"testsuite " + rows[0].suite + "  " + bar(c)})
	b.WriteString("## History\n\n")
	drawSeries(&b, l.series(past, pickSuite(rows[0].suite), c))
	drawProcesses(&b, headProcesses, rows, hist)

	if !split(rows) {
		for _, r := range rows {
			l.zoomCase(&b, base, r)
		}

		return b.String()
	}

	fmt.Fprintf(&b, "## Testcases\n\n- index: %s\n\n", link(slug.Make(rows[0].suiteShort)+"/"+IndexFile, base,
		filepath.Join(l.zoomCaseDir(rows[0]), IndexFile)))
	b.WriteString("| testcase | verdict | steps | bar | zoom |\n| -- | -- | -- | -- | -- |\n")

	for i, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s |\n", cell(r.testcase), verdictOf(r), len(r.view.Steps), progress(stepCount(r)),
			link(filepath.Base(l.zoomCaseFile(r, i)), base, l.zoomCaseFile(r, i)))
	}

	return b.String()
}

// zoomCaseText is 03-suites/<testsuite>/<NN>-<testcase>.md of a testsuite split by testcase: the testcase i as its
// section in a testsuite zoom.
func (l layout) zoomCaseText(r row, i int) string {
	base := dirOf(l.zoomCaseFile(r, i))
	var b strings.Builder
	fmt.Fprintf(&b, "# Testcase %s\n\n- testsuite: %s\n- zoom: %s · %s · %s\n\n", r.testcase,
		link(cell(r.suite), base, l.zoomSuite(r)), link(IndexFile, base, filepath.Join(l.zoomCaseDir(r), IndexFile)),
		link(l.pathOf(r), base, l.zoomPath(l.pathOf(r))), link(ZoomRunFile, base, l.zoomRun()))
	l.zoomCase(&b, base, r)

	return b.String()
}

// zoomCase is the section of one testcase in its testsuite zoom.
func (l layout) zoomCase(b *strings.Builder, base string, r row) {
	protocol := noValue

	if written(r) {
		protocol = link(protocolLink, base, r.protocol)
	}

	fmt.Fprintf(b, "## %s\n\n- verdict: %s\n- outcome: %s\n- steps: %s\n- scenario: %s\n", r.testcase, verdictOf(r),
		outcomeOf(r), progress(stepCount(r)), scenarioLink(base, r, r.view.Line))

	if !l.plan {
		fmt.Fprintf(b, "- protocol: %s · scheme: %s\n", protocol, link(wordScheme, base, l.caseScheme(r)))
	}

	for _, id := range traceIDs(r) {
		fmt.Fprintf(b, "- trace id: %s\n", traceRef(base, r, id))
	}

	b.WriteString("\n")
	lines := []string{r.testcase + "  " + progress(stepCount(r)) + "  " + verdictOf(r)}

	for i, s := range r.view.Steps {
		branch, indent := branchMid, indentMid

		if i == len(r.view.Steps)-1 {
			branch, indent = branchEnd, indentEnd
		}

		lines = append(lines, branch+fmt.Sprintf("%d %s  %s", s.Number, s.Name, verdict(s.Mark, s.Code)))
		lines = append(lines, stepLines(indent+"   ", s)...)
	}

	fence(b, lines)
	drawProcesses(b, "### Processes of "+caseID(r.testcase), []row{r}, nil)
	b.WriteString("| step | executor | method | url / database | outcome | code | trace | scenario |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- |\n")

	for _, s := range r.view.Steps {
		trace := noValue

		if s.TraceID != "" {
			trace = traceRef(base, r, s.TraceID)
		}

		fmt.Fprintf(b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n", s.Number, orNone(s.Type), orNone(cell(s.Method)),
			codeCell(orEmpty(s.URL, s.Database)), s.Mark, orNone(s.Code), trace, scenarioLink(base, r, s.Line))
	}

	b.WriteString("\n")
}

// outcomeOf is the outcome of the protocol of the row, «no run» of a testcase no run has.
func outcomeOf(r row) string {
	if r.state == stateNoRun {
		return stateNoRun
	}

	return r.outcome
}

func orEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}
