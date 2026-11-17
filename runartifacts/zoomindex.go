package runartifacts

import (
	"fmt"
	"strings"
)

// zoomIndexText is schemes/INDEX.md: the zoom set top-down — the run, the system, each path with its testsuites —
// then the schemes of the system and of each path.
func (l layout) zoomIndexText() string {
	base := l.zoomDir()
	c := countOf(l.rows)
	var b strings.Builder
	b.WriteString("# Schemes\n\n")
	fmt.Fprintf(&b, "- %s — run · %s\n", link(ZoomRunFile, base, l.zoomRun()), bar(c))
	fmt.Fprintf(&b, "- %s — system · %s\n", link(ZoomSystemFile, base, l.zoomSystem()), bar(c))

	for _, folder := range l.order {
		name := l.names[folder]
		fmt.Fprintf(&b, "  - %s — path · %s\n", link(ZoomPathsDir+"/"+name+".md", base, l.zoomPath(name)),
			bar(countOf(l.rowsOf(folder))))

		for _, s := range suitesOf(l.rowsOf(folder)) {
			fmt.Fprintf(&b, "    - %s — testsuite %s · %s\n", link(relTarget(base, l.zoomSuite(s[0])), base, l.zoomSuite(s[0])),
				cell(s[0].suite), bar(countOf(s)))
		}
	}

	fmt.Fprintf(&b, "\n- %s · %s\n", link(ZoomPathsDir+"/"+IndexFile, base, l.zoomLevel(ZoomPathsDir)),
		link(ZoomSuitesDir+"/"+IndexFile, base, l.zoomLevel(ZoomSuitesDir)))

	if l.plan {
		return b.String()
	}

	fmt.Fprintf(&b, "\n## Schemes of the run\n\n- %s\n", link(SystemFile, base, l.systemScheme()))

	for _, folder := range l.order {
		name := l.names[folder]
		fmt.Fprintf(&b, "- %s\n", link(name+"/"+PathFile, base, l.pathScheme(name)))
	}

	fmt.Fprintf(&b, "\n%s · %s · %s\n", link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()),
		link(IndexFile, base, l.index()))

	return b.String()
}

// zoomPathsIndexText is 02-paths/INDEX.md: every path with its bar and its testsuites.
func (l layout) zoomPathsIndexText() string {
	file := l.zoomLevel(ZoomPathsDir)
	base := dirOf(file)
	var b strings.Builder
	fmt.Fprintf(&b, "# Paths\n\n- zoom: %s · %s\n\n| path | folder | testsuites | verdict | bar |\n| -- | -- | -- | -- | -- |\n",
		link(ZoomRunFile, base, l.zoomRun()), link(IndexFile, base, l.zoomIndex()))

	for _, folder := range l.order {
		name := l.names[folder]
		rows := l.rowsOf(folder)
		c := countOf(rows)
		fmt.Fprintf(&b, "| %s | `%s` | %d | %s | %s |\n", link(cell(name), base, l.zoomPath(name)), folder, len(suitesOf(rows)),
			c.mark(), progress(c))
	}

	return b.String()
}

// zoomSuitesIndexText is 03-suites/INDEX.md: every testsuite with its path, its testcases and its bar.
func (l layout) zoomSuitesIndexText() string {
	file := l.zoomLevel(ZoomSuitesDir)
	base := dirOf(file)
	var b strings.Builder
	fmt.Fprintf(&b, "# Testsuites\n\n- zoom: %s · %s\n\n| testsuite | path | testcases | verdict | bar |\n| -- | -- | -- | -- | -- |\n",
		link(ZoomRunFile, base, l.zoomRun()), link(IndexFile, base, l.zoomIndex()))

	for _, s := range suitesOf(l.rows) {
		c := countOf(s)
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s |\n", link(cell(s[0].suite), base, l.zoomSuite(s[0])), cell(l.pathOf(s[0])),
			len(s), c.mark(), progress(c))
	}

	return b.String()
}

// zoomCasesIndexText is 03-suites/<testsuite>/INDEX.md of a testsuite split by testcase: every testcase with its file
// and the bar of its steps.
func (l layout) zoomCasesIndexText(rows []row) string {
	base := l.zoomCaseDir(rows[0])
	var b strings.Builder
	fmt.Fprintf(&b, "# Testcases of %s\n\n- zoom: %s · %s\n\n| testcase | verdict | steps | bar |\n| -- | -- | -- | -- |\n",
		rows[0].suite, link(relTarget(base, l.zoomSuite(rows[0])), base, l.zoomSuite(rows[0])),
		link(ZoomSuitesDir+"/"+IndexFile, base, l.zoomLevel(ZoomSuitesDir)))

	for i, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", link(cell(r.testcase), base, l.zoomCaseFile(r, i)), verdictOf(r),
			len(r.view.Steps), progress(stepCount(r)))
	}

	return b.String()
}
