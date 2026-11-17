package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// The words of the dynamics of a testcase against the previous run of the set that has it.
const (
	dynNew       = "new"
	dynImproved  = "improved"
	dynWorse     = "worse"
	dynSame      = "same"
	headDynamics = "## Dynamics"
	previousKey  = "- previous run: "
	noPrevious   = "none"
)

// outcomeRank orders the outcomes from the best: ok, partial, failed, not-run.
var outcomeRank = map[string]int{venom.ProtocolOutcomeOK: 3, venom.ProtocolOutcomePartial: 2,
	venom.ProtocolOutcomeFailed: 1, venom.ProtocolOutcomeNotRun: 0}

// previousOf is the last earlier run of the set that has the testcase name and the outcome of the testcase in it.
func (l layout) previousOf(name string) (string, string, bool) {
	run, ok := lastRunOf(l.past, name)
	if !ok {
		return "", "", false
	}

	return run.name, pastOutcome(run.cases[name]), true
}

// pastOutcome is the outcome of a testcase of an earlier run: the one its REPORT.md names in ## Dynamics, else the one
// its verdict and the share of its steps tell — not-run for a testcase that did not end, ok when every step passed,
// partial when some did, failed otherwise.
func pastOutcome(c caseRun) string {
	switch {
	case c.outcome != "":
		return c.outcome

	case c.mark != venom.MarkPassed && c.mark != venom.MarkFailed:
		return venom.ProtocolOutcomeNotRun

	case c.ok == 100:
		return venom.ProtocolOutcomeOK

	case c.ok > 0:
		return venom.ProtocolOutcomePartial

	default:
		return venom.ProtocolOutcomeFailed
	}
}

// dynamicsVerdict is improved, worse or same of the outcome now against the outcome before.
func dynamicsVerdict(before, now string) string {
	switch {
	case outcomeRank[now] > outcomeRank[before]:
		return dynImproved

	case outcomeRank[now] < outcomeRank[before]:
		return dynWorse

	default:
		return dynSame
	}
}

// dynamicsOf is the testcase against the previous run of the set that has it: «failed → ok · improved · 2026-11/17-9»,
// «ok → pending · 2026-11/17-9» while it has not started, «new» when no earlier run has it.
func (l layout) dynamicsOf(r row) string {
	run, before, ok := l.previousOf(r.testcase)

	switch {
	case !ok:
		return dynNew

	case r.state == statePending:
		return fmt.Sprintf("%s → %s · %s", before, statePending, run)

	default:
		return fmt.Sprintf("%s → %s · %s · %s", before, r.outcome, dynamicsVerdict(before, r.outcome), run)
	}
}

// previousTotal is the counts of the outcomes against the last earlier run of the set: «2026-11/17-9 · ok 0 → 1 (+1) ·
// partial 0 → 0 (0) · failed 2 → 0 (-2) · not-run 0 → 1 (+1)», «none» with no earlier run.
func (l layout) previousTotal(now count) string {
	if len(l.past) == 0 {
		return noPrevious
	}

	run := l.past[len(l.past)-1]
	before := run.total
	parts := []string{run.name}

	for _, o := range []struct {
		name        string
		before, now int
	}{
		{venom.ProtocolOutcomeOK, before.ok, now.ok},
		{venom.ProtocolOutcomePartial, before.partial, now.partial},
		{venom.ProtocolOutcomeFailed, before.notOK, now.notOK},
		{venom.ProtocolOutcomeNotRun, before.notRun, now.notRun},
	} {
		parts = append(parts, fmt.Sprintf("%s %d → %d (%+d)", o.name, o.before, o.now, o.now-o.before))
	}

	return strings.ReplaceAll(strings.Join(parts, " · "), "(+0)", "(0)")
}

// writeDynamics writes the ## Dynamics section of REPORT.md: every testcase with the previous run that has it, its
// outcome there and now, and improved, worse, same or new.
func (l layout) writeDynamics(b *strings.Builder) {
	b.WriteString("\n" + headDynamics + "\n\n| testcase | previous run | previous | current | dynamics |\n| -- | -- | -- | -- | -- |\n")

	for _, r := range l.rows {
		run, before, ok := l.previousOf(r.testcase)
		verdict := dynNew

		switch {
		case !ok:
			run, before = noValue, noValue

		case r.state == statePending:
			verdict = statePending

		default:
			verdict = dynamicsVerdict(before, r.outcome)
		}

		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", cell(r.testcase), run, before, r.outcome, verdict)
	}
}

// readDynamics puts the outcome now of a row of the ## Dynamics section — testcase, previous run, previous, current,
// dynamics — on its testcase in cases.
func readDynamics(cases map[string]caseRun, cells []string) {
	if len(cells) < 5 {
		return
	}

	name := unescape(cells[0])
	c := cases[name]
	c.outcome = cells[3]
	cases[name] = c
}
