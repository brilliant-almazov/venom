package runartifacts

import (
	"fmt"
	"strings"
)

// zoomRunText is 00-run.md: the run total with its bar, its paths and testsuites with theirs, and the history of every
// earlier run of the same set.
func (l layout) zoomRunText(past []pastRun) string {
	base := l.zoomDir()
	c := countOf(l.rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# Run %s\n\n", l.thisRun())
	fmt.Fprintf(&b, "- testcases: %s\n- zoom: %s · %s\n- run: %s · %s · %s\n\n", c, link(IndexFile, base, l.zoomIndex()),
		link(ZoomSystemFile, base, l.zoomSystem()), link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()),
		link(IndexFile, base, l.index()))

	lines := []string{"run " + l.thisRun() + "  " + bar(c)}

	for i, folder := range l.order {
		branch, indent := branchMid, indentMid

		if i == len(l.order)-1 {
			branch, indent = branchEnd, indentEnd
		}

		lines = append(lines, branch+"path "+l.names[folder]+"  "+bar(countOf(l.rowsOf(folder))))
		suites := suitesOf(l.rowsOf(folder))

		for j, s := range suites {
			leaf := branchMid

			if j == len(suites)-1 {
				leaf = branchEnd
			}

			lines = append(lines, indent+leaf+"testsuite "+s[0].suite+"  "+bar(countOf(s)))
		}
	}

	fence(&b, lines)
	b.WriteString("## History\n\n")
	drawSeries(&b, seriesOf(past, pickTotal, l.thisRun(), c))
	b.WriteString("## Paths\n\n| path | verdict | bar | zoom |\n| -- | -- | -- | -- |\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(name), pc.mark(), progress(pc), link(name, base, l.zoomPath(name)))
	}

	return b.String()
}
