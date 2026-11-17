package runartifacts

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ovh/venom"
)

// The headings of the tables of REPORT.md a past run is read from.
const (
	headPaths  = "## Paths"
	headSuites = "## Testsuites"
	verdictKey = "- verdict: "
	casesKey   = "- testcases: "
)

// tallyOf is passed of all in a verdict: ❌ 80/88.
var tallyOf = regexp.MustCompile(`(\d+)/(\d+)`)

// sharesOf are the shares of ok, partial and failed of a block: ok 50% · partial 25% · failed 25%.
var sharesOf = regexp.MustCompile(`ok (\d+)% · partial (\d+)% · failed (\d+)%`)

// readReport is the past run name as its REPORT.md tells it: the verdict line, the verdict of every path and testsuite.
func readReport(name, body string) pastRun {
	run := pastRun{name: name, paths: map[string]count{}, suites: map[string]count{}}
	var table map[string]count
	verdict := ""

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, verdictKey):
			verdict = line

		case strings.HasPrefix(line, casesKey):
			run.total = countFrom(verdict + " " + line)

		case strings.HasPrefix(line, headPaths):
			table = run.paths

		case strings.HasPrefix(line, headSuites):
			table = run.suites

		case strings.HasPrefix(line, "## "):
			table = nil

		case table != nil && strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| -- "):
			cells := splitCells(line)

			if len(cells) > 3 && strings.Contains(cells[3], "/") {
				table[unescape(cells[0])] = countFrom(cells[3] + " " + cells[2])
			}

		default:
		}
	}

	return run
}

// splitCells are the cells of a table row, \| kept inside a cell.
func splitCells(line string) []string {
	row := strings.ReplaceAll(trimRow(line), `\|`, "\x00")
	cells := strings.Split(row, "|")

	for i := range cells {
		cells[i] = strings.ReplaceAll(strings.TrimSpace(cells[i]), "\x00", `\|`)
	}

	return cells
}

func unescape(s string) string {
	return strings.ReplaceAll(s, `\|`, "|")
}

// partOf is the testcases of all a share in % stands for.
func partOf(pct string, all int) int {
	n, _ := strconv.Atoi(pct)

	return (n*all + 50) / 100
}

// countFrom is the count of a verdict: passed of all, failed when it is marked so, the rest neither; ok, partial and
// failed from the shares it carries, ok passed when it carries none.
func countFrom(verdict string) count {
	m := tallyOf.FindStringSubmatch(verdict)
	if m == nil {
		return count{}
	}

	passed, _ := strconv.Atoi(m[1])
	all, _ := strconv.Atoi(m[2])
	c := count{all: all, passed: passed, ok: passed, notOK: all - passed}

	if s := sharesOf.FindStringSubmatch(verdict); s != nil {
		c.ok, c.partial = partOf(s[1], all), partOf(s[2], all)
		c.notOK = all - c.ok - c.partial
	}

	switch {
	case strings.Contains(verdict, venom.MarkFailed):
		c.failed = all - passed

	default:
		c.other = all - passed
	}

	return c
}
