package runartifacts

import (
	"fmt"
	"strings"
)

// headProcesses is the section of the processes of a zoom, the table a later run reads the history of its points from.
const headProcesses = "## Processes"

// drawProcesses is the section head of the processes the rows went through: each binary with its bar, under it every
// entry and exit with the testcases that cover it and their bar, then the table with the history of every point in
// the same file of the earlier runs (hist, oldest first).
func drawProcesses(b *strings.Builder, head string, rows []row, hist []map[string]count) {
	procs := processesOf(rows)
	b.WriteString(head + "\n\n")

	if len(procs) == 0 {
		b.WriteString("no saved trace of these testcases names a process\n\n")
		return
	}

	var lines []string

	for _, pr := range procs {
		lines = append(lines, pr.name+"  "+bar(countOf(pr.rows)))

		for i, pt := range pr.points {
			branch := branchMid

			if i == len(pr.points)-1 {
				branch = branchEnd
			}

			lines = append(lines, branch+pad(pt.dir, 3)+" "+pt.name+"  "+bar(countOf(pt.rows))+red(failedOf(pt))+
				"  · "+caseIDs(pt.rows))
		}
	}

	fence(b, lines)
	b.WriteString("| process | direction | point | testcases | verdict | bar | failed spans | cases | history |\n")
	b.WriteString("| -- | -- | -- | -- | -- | -- | -- | -- | -- |\n")

	for _, pr := range procs {
		for _, pt := range pr.points {
			c := countOf(pt.rows)
			fmt.Fprintf(b, "| %s | %s | %s | %d | %s | %s | %d | %s | %s |\n", cell(pr.name), pt.dir, cell(pt.name), c.all,
				c.mark(), progress(c), pt.failed, cell(caseIDs(pt.rows)), pointHistory(hist, pt.key(pr.name), c))
		}
	}

	b.WriteString("\n")
}

// failedOf is a count with the failed spans of a point, so red marks it.
func failedOf(pt *endpoint) count {
	return count{failed: pt.failed}
}

// caseIDs are the row ids of the testcases: the first word of each name.
func caseIDs(rows []row) string {
	ids := make([]string, 0, len(rows))

	for _, r := range rows {
		ids = append(ids, caseID(r.testcase))
	}

	return strings.Join(ids, " ")
}

// caseID is the row id of a testcase: the first word of its name.
func caseID(name string) string {
	return strings.SplitN(name, " ", 2)[0]
}

// pointHistory is the ok share of a point in every earlier run that has it, then in this run, with Δ p.p. of this run
// against the run before: 40 % → 50 % → 91 % (Δ +41.0).
func pointHistory(hist []map[string]count, key string, now count) string {
	var shares []string
	var prev *count

	for _, h := range hist {
		if c, ok := h[key]; ok {
			shares = append(shares, fmt.Sprintf("%d %%", okShare(c)))
			prev = &c
		}
	}

	shares = append(shares, fmt.Sprintf("%d %%", okShare(now)))
	text := strings.Join(shares, " → ")

	if prev != nil {
		text += fmt.Sprintf(" (Δ %+.1f)", share(now)-share(*prev))
	}

	return text
}
