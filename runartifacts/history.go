package runartifacts

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// runsDir is the folder of the runs of a set: <set>/runs/<YYYY-MM>/<DD-n>.
const runsDir = "runs"

// pastRun is an earlier run of the same set as its REPORT.md tells it: the run total, each path and each testsuite.
type pastRun struct {
	name, dir string
	total     count
	paths     map[string]count
	suites    map[string]count
}

// pastRuns are the runs of the set before the run folder dir, oldest first: every sibling folder <YYYY-MM>/<DD-n>
// under runs/ with a REPORT.md, ordered by its numbers; none when dir is not under runs/.
func pastRuns(dir string) []pastRun {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}

	root := filepath.Dir(filepath.Dir(abs))

	if filepath.Base(root) != runsDir {
		return nil
	}

	self := runName(root, abs)
	reports, _ := filepath.Glob(filepath.Join(root, "*", "*", ReportFile))
	var runs []pastRun

	for _, report := range reports {
		name := runName(root, filepath.Dir(report))

		if !runBefore(name, self) {
			continue
		}

		body, err := os.ReadFile(report)
		if err != nil {
			continue
		}

		run := readReport(name, string(body))
		run.dir = filepath.Dir(report)
		runs = append(runs, run)
	}

	sort.SliceStable(runs, func(i, j int) bool { return runBefore(runs[i].name, runs[j].name) })

	return runs
}

func runName(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return dir
	}

	return filepath.ToSlash(rel)
}

// numbers are the runs of digits of a run name.
var numbers = regexp.MustCompile(`\d+`)

// runBefore: run a comes before run b by the numbers of their names, 2026-11/17-9 before 2026-11/17-13.
func runBefore(a, b string) bool {
	na, nb := numbers.FindAllString(a, -1), numbers.FindAllString(b, -1)

	for i := 0; i < len(na) && i < len(nb); i++ {
		x, _ := strconv.Atoi(na[i])
		y, _ := strconv.Atoi(nb[i])

		if x != y {
			return x < y
		}
	}

	if len(na) != len(nb) {
		return len(na) < len(nb)
	}

	return a < b
}

// history are the earlier runs of the set, read once per run folder: they do not change while it runs.
func (w *Writer) history() *archive {
	if w.past == nil {
		w.past = &archive{runs: pastRuns(w.dir), points: map[string][]map[string]count{}}
	}

	return w.past
}

// trimRow is a table row of REPORT.md without its outer bars.
func trimRow(line string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "|"), "|")
}
