package runartifacts

import (
	"fmt"
	"strings"
)

// writeSystem writes schemes/SYSTEM.md: the whole system of the run, every path with every case and its transitions,
// then the paths and the checked cases.
func (l layout) writeSystem() error {
	file := l.systemScheme()
	base := dirOf(file)
	c := countOf(l.rows)
	var b strings.Builder
	b.WriteString("# Scheme of the system\n\n")
	fmt.Fprintf(&b, "- verdict: %s\n- testcases: %s\n- run: %s · %s · %s\n\n", c.mark(), c, link(IndexFile, base, l.index()),
		link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()))
	b.WriteString("```text\nsystem · " + c.mark() + " · " + c.share() + "\n")

	for i, folder := range l.order {
		branch, indent := branchMid, indentMid

		if i == len(l.order)-1 {
			branch, indent = branchEnd, indentEnd
		}

		rows := l.rowsOf(folder)
		b.WriteString(branch + l.names[folder] + " · " + countOf(rows).mark() + " · " + countOf(rows).share() + "\n")
		drawCases(&b, indent, rows)
	}

	b.WriteString("```\n\n## Paths\n\n| path | folder | testcases | verdict | scheme |\n| -- | -- | -- | -- | -- |\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s |\n", cell(name), folder, pc, pc.mark(), link(name, base, l.pathScheme(name)))
	}

	b.WriteString("\n")
	l.checkedCases(&b, base, l.rows)

	return writeFile(file, []byte(b.String()))
}

// writePath writes schemes/<path>/PATH.md: the cases of one path with their transitions, then the checked cases.
func (l layout) writePath(name string) error {
	file := l.pathScheme(name)
	base := dirOf(file)
	folder := ""

	for f, n := range l.names {
		if n == name {
			folder = f
		}
	}

	rows := l.rowsOf(folder)
	c := countOf(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "# Scheme of the path %s\n\n", name)
	fmt.Fprintf(&b, "- folder: `%s`\n- verdict: %s\n- testcases: %s\n- system: %s · %s\n\n", folder, c.mark(), c,
		link(SystemFile, base, l.systemScheme()), link(IndexFile, base, l.index()))
	b.WriteString("```text\n" + name + " · " + c.mark() + " · " + c.share() + "\n")
	drawCases(&b, "", rows)
	b.WriteString("```\n\n")
	l.checkedCases(&b, base, rows)

	return writeFile(file, []byte(b.String()))
}

// checkedCases is the table of the checked cases of a scheme: how each works — the request, the expected behaviour,
// the transitions — and its links.
func (l layout) checkedCases(b *strings.Builder, base string, rows []row) {
	b.WriteString("## Checked cases\n\n")
	b.WriteString("| testcase | path | verdict | request | expected | transitions | scenario | protocol | scheme | trace |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- | -- | -- |\n")

	for _, r := range rows {
		protocol := noValue

		if written(r) {
			protocol = link(protocolLink, base, r.protocol)
		}

		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", cell(r.testcase), cell(l.pathOf(r)),
			verdictOf(r), codeCell(r.view.Request), one(expectedOf(r)), transitionsOf(r), scenarioLink(base, r, r.view.Line),
			protocol, link(wordScheme, base, l.caseScheme(r)), traceCell(base, r))
	}
}

// expectedOf is the expected behaviour of the testcase, «not declared in info» when the scenario gives none.
func expectedOf(r row) string {
	if len(r.view.Expected) == 0 {
		return notDeclared
	}

	return strings.Join(r.view.Expected, "; ")
}

// transitionsOf is every transition of the testcase as its step number and mark: 1 ✅ · 2 ❌ · 3 ⏸.
func transitionsOf(r row) string {
	words := make([]string, 0, len(r.view.Steps))

	for _, s := range r.view.Steps {
		words = append(words, fmt.Sprintf("%d %s", s.Number, s.Mark))
	}

	if len(words) == 0 {
		return noValue
	}

	return strings.Join(words, " · ")
}

// codeCell is a request in a table cell, «—» when there is none.
func codeCell(s string) string {
	if s == "" {
		return noValue
	}

	return "`" + strings.ReplaceAll(one(s), "`", "'") + "`"
}

const notDeclared = "not declared in info"
