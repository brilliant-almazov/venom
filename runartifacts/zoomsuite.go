package runartifacts

import (
	"fmt"
	"strings"
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
	drawSeries(&b, seriesOf(past, pickSuite(rows[0].suite), l.thisRun(), c))
	drawProcesses(&b, headProcesses, rows, hist)

	for _, r := range rows {
		l.zoomCase(&b, base, r)
	}

	return b.String()
}

// zoomCase is the section of one testcase in its testsuite zoom.
func (l layout) zoomCase(b *strings.Builder, base string, r row) {
	protocol := noValue

	if written(r) {
		protocol = link(protocolLink, base, r.protocol)
	}

	fmt.Fprintf(b, "## %s\n\n- verdict: %s\n- outcome: %s\n- steps: %s\n- scenario: %s\n- protocol: %s · scheme: %s\n",
		r.testcase, verdictOf(r), r.outcome, progress(stepCount(r)), scenarioLink(base, r, r.view.Line), protocol,
		link(wordScheme, base, l.caseScheme(r)))

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
	drawProcesses(b, "### Processes of "+strings.SplitN(r.testcase, " ", 2)[0], []row{r}, nil)
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

func orEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}
