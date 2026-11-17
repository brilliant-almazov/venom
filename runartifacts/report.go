package runartifacts

import (
	"fmt"
	"strings"
	"time"

	"github.com/ovh/venom"
)

// count is how many testcases passed, failed and did not pass or fail (skipped, interrupted, not run, pending).
type count struct {
	all, passed, failed, other int
}

func countOf(rows []row) count {
	c := count{all: len(rows)}

	for _, r := range rows {
		switch {
		case passed(r):
			c.passed++

		case failed(r):
			c.failed++

		default:
			c.other++
		}
	}

	return c
}

// mark is ✅ when every testcase passed, ❌ when one failed, else ⏸; then passed of all.
func (c count) mark() string {
	switch {
	case c.failed > 0:
		return fmt.Sprintf("%s %d/%d", venom.MarkFailed, c.passed, c.all)

	case c.other > 0 || c.all == 0:
		return fmt.Sprintf("%s %d/%d", venom.MarkNotRun, c.passed, c.all)

	default:
		return fmt.Sprintf("%s %d/%d", venom.MarkPassed, c.passed, c.all)
	}
}

func (c count) String() string {
	return fmt.Sprintf("%d testcases · %s %d · %s %d · %s %d", c.all, venom.MarkPassed, c.passed, venom.MarkFailed, c.failed,
		venom.MarkNotRun, c.other)
}

// writeReport writes REPORT.md: the state of the run, its counts, every path and every trace, with the links.
func (l layout) writeReport(w *Writer) error {
	base := dirOf(l.report())
	c := countOf(l.rows)
	var b strings.Builder
	b.WriteString("# Run report\n\n")
	fmt.Fprintf(&b, "- verdict: %s\n- testcases: %s\n", c.mark(), c)
	fmt.Fprintf(&b, "- start: %s\n- end: %s\n", w.start.UTC().Format(time.RFC3339), endOf(w))

	if w.runErr != "" {
		fmt.Fprintf(&b, "- run ended early: %s\n", one(w.runErr))
	}

	fmt.Fprintf(&b, "- links: %s · %s · %s · %s\n", link(IndexFile, base, l.index()), link(ProblemsFile, base, l.problems()),
		link(SystemFile, base, l.systemScheme()), link(RawDir+"/", base, w.RawPath()))
	b.WriteString("\n## Paths\n\n| path | folder | testcases | verdict | scheme |\n| -- | -- | -- | -- | -- |\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s |\n", cell(name), folder, pc, pc.mark(), link(name, base, l.pathScheme(name)))
	}

	b.WriteString("\n## Traces\n\n| trace id | testcase | span tree |\n| -- | -- | -- |\n")

	for _, r := range l.rows {
		for _, id := range traceIDs(r) {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", id, link(cell(r.testcase), base, r.protocol), traceRef(base, r, id))
		}
	}

	return writeFile(l.report(), []byte(b.String()))
}

// endOf is the end of the run, «under way» while it runs.
func endOf(w *Writer) string {
	if w.end.IsZero() {
		return "under way"
	}

	return w.end.UTC().Format(time.RFC3339)
}
