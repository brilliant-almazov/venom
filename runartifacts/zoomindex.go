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

	fmt.Fprintf(&b, "\n## Schemes of the run\n\n- %s\n", link(SystemFile, base, l.systemScheme()))

	for _, folder := range l.order {
		name := l.names[folder]
		fmt.Fprintf(&b, "- %s\n", link(name+"/"+PathFile, base, l.pathScheme(name)))
	}

	fmt.Fprintf(&b, "\n%s · %s · %s\n", link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()),
		link(IndexFile, base, l.index()))

	return b.String()
}
