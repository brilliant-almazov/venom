package runartifacts

import (
	"fmt"
	"strings"
)

// zoomPathText is 02-paths/<path>.md: the path with its bar and history, then every testsuite of it with its bar, its
// history and its testcases.
func (l layout) zoomPathText(folder string, past []pastRun, hist []map[string]count) string {
	name := l.names[folder]
	file := l.zoomPath(name)
	base := dirOf(file)
	rows := l.rowsOf(folder)
	c := countOf(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# Path %s\n\n", name)
	fmt.Fprintf(&b, "- folder: `%s`\n- testcases: %s\n- zoom: %s · %s · %s\n", folder, c,
		link(ZoomRunFile, base, l.zoomRun()), link(ZoomSystemFile, base, l.zoomSystem()), link(IndexFile, base, l.zoomIndex()))

	if !l.plan {
		fmt.Fprintf(&b, "- scheme: %s\n", link(PathFile, base, l.pathScheme(name)))
	}

	b.WriteString("\n")

	lines := []string{"path " + name + "  " + bar(c)}
	suites := suitesOf(rows)

	for i, s := range suites {
		branch := branchMid

		if i == len(suites)-1 {
			branch = branchEnd
		}

		lines = append(lines, branch+"testsuite "+s[0].suite+"  "+bar(countOf(s)))
	}

	fence(&b, lines)
	b.WriteString("## History\n\n")
	drawSeries(&b, l.series(past, pickPath(name), c))
	drawProcesses(&b, headProcesses, rows, hist)

	for _, s := range suites {
		sc := countOf(s)
		fmt.Fprintf(&b, "## Testsuite %s\n\n- testcases: %s\n- zoom: %s\n\n", s[0].suite, sc,
			link(relTarget(base, l.zoomSuite(s[0])), base, l.zoomSuite(s[0])))

		lines := []string{"testsuite " + s[0].suite + "  " + bar(sc)}

		for i, r := range s {
			branch := branchMid

			if i == len(s)-1 {
				branch = branchEnd
			}

			lines = append(lines, branch+r.testcase+"  "+progress(stepCount(r))+"  "+verdictOf(r))
		}

		fence(&b, lines)
		drawSeries(&b, l.series(past, pickSuite(s[0].suite), sc))
	}

	return b.String()
}
