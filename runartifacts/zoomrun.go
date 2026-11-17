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
	fmt.Fprintf(&b, "# %s\n\n", upper(l.title()))
	fmt.Fprintf(&b, "- testcases: %s\n- zoom: %s · %s · %s · %s\n", c, link(IndexFile, base, l.zoomIndex()),
		link(ZoomSystemFile, base, l.zoomSystem()), link(ZoomPathsDir+"/", base, l.zoomLevel(ZoomPathsDir)),
		link(ZoomSuitesDir+"/", base, l.zoomLevel(ZoomSuitesDir)))
	l.runLinks(&b, base)
	b.WriteString("\n")

	lines := []string{l.title() + "  " + bar(c)}

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
	drawSeries(&b, l.series(past, pickTotal, c))
	b.WriteString("## Paths\n\n| path | verdict | bar | zoom |\n| -- | -- | -- | -- |\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(name), pc.mark(), progress(pc), link(name, base, l.zoomPath(name)))
	}

	return b.String()
}
