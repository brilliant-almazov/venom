package runartifacts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// indexHeader is the table of protocols/INDEX.md, one row per testcase.
const indexHeader = "| suite | testcase | verdict | trace | link | scenario | scheme | path | previous |\n" +
	"| -- | -- | -- | -- | -- | -- | -- | -- | -- |\n"

// noTree is the trace cell of a call whose trace id has no span tree in Tempo.
const noTree = "trace not saved"

// The texts of a link to a protocol and to a scheme.
const (
	protocolLink = "protocol"
	wordScheme   = "scheme"
)

// sortRows puts the testcases in testsuite file order, then in their order in the testsuite.
func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].suiteFile != rows[j].suiteFile {
			return rows[i].suiteFile < rows[j].suiteFile
		}

		return rows[i].index < rows[j].index
	})
}

// writeIndex writes protocols/INDEX.md: one row per testcase with its verdict, trace, protocol, scenario line, scheme
// and its state against the previous run.
func (l layout) writeIndex() error {
	base := filepath.Dir(l.index())
	var b strings.Builder
	b.WriteString("# Protocols\n\n")
	b.WriteString(link(ReportFile, base, l.report()) + " · " + link(ProblemsFile, base, l.problems()) + " · " +
		link(SystemFile, base, l.systemScheme()) + "\n\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "- path %s · %s · %s\n", link(name, base, l.pathScheme(name)), pc.mark(), pc.share())
	}

	c := countOf(l.rows)
	fmt.Fprintf(&b, "- system %s · %s · %s\n%s%s\n\n%s", link(SystemFile, base, l.systemScheme()), c.mark(), c.share(),
		previousKey, l.previousTotal(c), indexHeader)

	for _, r := range l.rows {
		protocol := noValue

		if written(r) {
			protocol = link(protocolLink, base, r.protocol)
		}

		path := l.pathOf(r)
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", cell(r.suite), cell(r.testcase), verdictOf(r),
			traceCell(base, r), protocol, scenarioLink(base, r, r.view.Line), link(wordScheme, base, l.caseScheme(r)),
			link(cell(path), base, l.pathScheme(path)), l.dynamicsOf(r))
	}

	return writeFile(l.index(), []byte(b.String()))
}

// verdict is the mark with the answer code, when the call had one.
func verdict(mark, code string) string {
	if code == "" {
		return mark
	}

	return mark + " " + code
}

// traceCell is the link to the span tree of the call, «trace not saved <trace_id>» when Tempo has none, «—» for
// a call with no trace id (the monolith).
func traceCell(from string, r row) string {
	switch {
	case r.trace != "":
		return link(short(r.view.TraceID), from, r.trace)

	case r.view.TraceID != "":
		return noTree + " `" + r.view.TraceID + "`"

	default:
		return "—"
	}
}

// short is the first 8 characters of a trace id.
func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}

// link is a markdown link to file relative to the folder from.
func link(text, from, file string) string {
	return "[" + text + "](" + relTarget(from, file) + ")"
}

// relTarget is the target of a link to file relative to the folder from.
func relTarget(from, file string) string {
	rel, err := filepath.Rel(from, file)
	if err != nil {
		rel = file
	}

	return strings.ReplaceAll(filepath.ToSlash(rel), " ", "%20")
}

func cell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
