package runartifacts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ovh/venom"
)

// The words of the problems of a run by priority.
const (
	headPriority = "## Problems by priority"
	problemHead  = "| problem | testcase | kind | criticality | verdict | " + wordFixer + " |\n| -- | -- | -- | -- | -- | -- |\n"
)

// problemID is the index of the problem of a testcase, the same in every run: P-<testcase name as a file name>.
func problemID(r row) string {
	return "P-" + venom.ProtocolFileName(r.testcase)
}

// levelOf is a declared level, «not set» for a testcase that declares none.
func levelOf(name string) string {
	if name == "" {
		return venom.ProblemNotSet
	}

	return name
}

// problemsOf are the real defects of the run by priority: the testcases that ended failed on the product — not one
// skipped, interrupted, not run or pending, not one red for the stand set up wrong (venom.ProtocolView.Stand) —, by the
// kind of their scenario, then by their criticality, each from the first to fix as venomrc.base.yaml or a .venomrc sets
// them, «not set» last; testcase order on a tie.
func problemsOf(rows []row) []row {
	var out []row

	for _, r := range rows {
		if failed(r) && r.view.Stand == "" {
			out = append(out, r)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := venom.KindRank(levelOf(out[i].view.Kind)), venom.KindRank(levelOf(out[j].view.Kind))

		if ki != kj {
			return ki < kj
		}

		return venom.CriticalityRank(levelOf(out[i].view.Criticality)) < venom.CriticalityRank(levelOf(out[j].view.Criticality))
	})

	return out
}

// problemLine is the problem of a testcase that did not pass as its protocol names it: its index, kind and criticality.
func problemLine(r row) string {
	return fmt.Sprintf("problem: %s · kind: %s · criticality: %s", problemID(r), venom.KindText(levelOf(r.view.Kind)),
		venom.CriticalityText(levelOf(r.view.Criticality)))
}

// writePriority writes the problems of the run by priority into REPORT.md.
func (l layout) writePriority(b *strings.Builder, base string) {
	b.WriteString("\n" + headPriority + "\n\n" + problemHead)
	problems := problemsOf(l.rows)

	for _, r := range problems {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", problemID(r), cell(r.testcase), cell(venom.KindText(levelOf(r.view.Kind))),
			cell(venom.CriticalityText(levelOf(r.view.Criticality))), verdictOf(r), l.fixCell(base, r))
	}

	if len(problems) == 0 {
		b.WriteString("| no problems | | | | | |\n")
	}
}
