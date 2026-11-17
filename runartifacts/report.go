package runartifacts

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ovh/venom"
)

// count is how many testcases passed, failed and did not pass or fail (skipped, interrupted, not run, pending), and
// how many protocols are ok, partial and failed; unrun are those of a scheme with no run that no earlier run has.
type count struct {
	all, passed, failed, other int
	ok, partial, notOK         int
	unrun                      int
}

func countOf(rows []row) count {
	c := count{all: len(rows)}

	for _, r := range rows {
		switch r.outcome {
		case venom.ProtocolOutcomeOK:
			c.ok++

		case venom.ProtocolOutcomePartial:
			c.partial++

		default:
			c.notOK++
		}

		if r.state == stateNoRun {
			c.unrun++
		}

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
	return fmt.Sprintf("%d testcases · %s %d · %s %d · %s %d · %s", c.all, venom.MarkPassed, c.passed, venom.MarkFailed, c.failed,
		venom.MarkNotRun, c.other, c.share())
}

// share is the share of the protocols ok, partial and failed in %: ok 50% · partial 25% · failed 25%.
func (c count) share() string {
	return fmt.Sprintf("%s %s · %s %s · %s %s", venom.ProtocolOutcomeOK, percent(c.ok, c.all), venom.ProtocolOutcomePartial,
		percent(c.partial, c.all), venom.ProtocolOutcomeFailed, percent(c.notOK, c.all))
}

// percent is n of all in whole %, 0% of none.
func percent(n, all int) string {
	if all == 0 {
		return "0%"
	}

	return fmt.Sprintf("%d%%", (n*100+all/2)/all)
}

// writeReport writes REPORT.md: the state of the run, its counts, every path and every trace, with the links.
func (l layout) writeReport(w *Writer) error {
	base := dirOf(l.report())
	c := countOf(l.rows)
	var b strings.Builder
	b.WriteString("# Run report\n\n")
	fmt.Fprintf(&b, "- verdict: %s · %s\n- testcases: %s\n", c.mark(), progress(c), c)
	fmt.Fprintf(&b, "- start: %s\n- end: %s\n", w.start.UTC().Format(time.RFC3339), endOf(w))

	if w.runErr != "" {
		fmt.Fprintf(&b, "- run ended early: %s\n", one(w.runErr))
	}

	fmt.Fprintf(&b, "- links: %s · %s · %s · %s · %s\n", link(IndexFile, base, l.index()), link(ProblemsFile, base, l.problems()),
		link(FixesDir+"/", base, l.fixesIndex()), link(SystemFile, base, l.systemScheme()), link(RawDir+"/", base, w.RawPath()))
	b.WriteString("\n## Paths\n\n| path | folder | testcases | verdict | bar | scheme |\n| -- | -- | -- | -- | -- | -- |\n")

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %s |\n", cell(name), folder, pc, pc.mark(), progress(pc),
			link(name, base, l.pathScheme(name)))
	}

	b.WriteString("\n## Testsuites\n\n| testsuite | protocols | testcases | verdict | bar | " + wordFixer +
		" |\n| -- | -- | -- | -- | -- | -- |\n")

	for _, s := range suitesOf(l.rows) {
		sc := countOf(s)

		fixes, err := l.fixLinks(base, s)
		if err != nil {
			return err
		}

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", cell(s[0].suite), link(filepath.Base(dirOf(s[0].protocol))+"/", base,
			dirOf(s[0].protocol)), sc, sc.mark(), progress(sc), fixes)
	}

	b.WriteString("\n## Schemes\n\n| slice | scheme | verdict | share |\n| -- | -- | -- | -- |\n")
	fmt.Fprintf(&b, "| system | %s | %s | %s |\n", link(SystemFile, base, l.systemScheme()), c.mark(), c.share())

	for _, folder := range l.order {
		name := l.names[folder]
		pc := countOf(l.rowsOf(folder))
		fmt.Fprintf(&b, "| path | %s | %s | %s |\n", link(name, base, l.pathScheme(name)), pc.mark(), pc.share())
	}

	for _, r := range l.rows {
		fmt.Fprintf(&b, "| case | %s | %s | %s |\n", link(cell(r.testcase), base, l.caseScheme(r)), verdictOf(r), stepShare(r))
	}

	b.WriteString("\n## Traces\n\n| trace id | testcase | span tree |\n| -- | -- | -- |\n")

	for _, r := range l.rows {
		for _, id := range traceIDs(r) {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", id, link(cell(r.testcase), base, r.protocol), traceRef(base, r, id))
		}
	}

	return writeFile(l.report(), []byte(b.String()))
}

// suitesOf are the rows of each testsuite, in testsuite file order.
func suitesOf(rows []row) [][]row {
	var suites [][]row
	at := map[string]int{}

	for _, r := range rows {
		i, ok := at[r.suiteShort]

		if !ok {
			i = len(suites)
			at[r.suiteShort] = i
			suites = append(suites, nil)
		}

		suites[i] = append(suites[i], r)
	}

	return suites
}

// stepShare is the share of the transitions of a testcase in %: ✅ 50% · ❌ 25% · ⏸ 25%.
func stepShare(r row) string {
	var ok, notOK, other int

	for _, s := range r.view.Steps {
		switch s.Mark {
		case venom.MarkPassed:
			ok++

		case venom.MarkFailed:
			notOK++

		default:
			other++
		}
	}

	all := len(r.view.Steps)

	return fmt.Sprintf("%s %s · %s %s · %s %s", venom.MarkPassed, percent(ok, all), venom.MarkFailed, percent(notOK, all),
		venom.MarkNotRun, percent(other, all))
}

// endOf is the end of the run, «under way» while it runs.
func endOf(w *Writer) string {
	if w.end.IsZero() {
		return "under way"
	}

	return w.end.UTC().Format(time.RFC3339)
}
