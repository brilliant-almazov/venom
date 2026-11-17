package runartifacts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// WritePlan writes the zoom set of the testsuites v read as written (venom.ReadPlan) into dir/schemes with no testcase
// run: the same files a run writes, each testcase with its steps as the scenario writes them and the steps of the user
// executors they call. The runs of the set under runs (<runs>/<YYYY-MM>/<DD-n>/REPORT.md) give the history of every
// block; each testcase takes its bar from the last of them that has it and the processes from the 01-system.md of that
// run; a testcase no run has gets «no run».
func WritePlan(ctx context.Context, v *venom.Venom, dir, runs string) error {
	w := New(dir)
	w.plan = true
	w.past = &archive{points: map[string][]map[string]count{}}

	if runs != "" {
		w.past.runs = runsUnder(runs, func(string) bool { return true })
	}

	points := map[string]map[string][]tempo.Point{}

	for i := range v.Tests.TestSuites {
		ts := &v.Tests.TestSuites[i]

		for j := range ts.TestCases {
			r := w.rowOf(ts, j)
			r.view = v.PlanView(ts, &ts.TestCases[j])
			r.state = stateNoRun
			run, ok := lastRunOf(w.past.runs, r.testcase)

			if !ok {
				continue
			}

			takeRun(r, run.cases[r.testcase])

			if _, read := points[run.dir]; !read {
				points[run.dir] = pointsOfRun(run.dir)
			}

			r.points = points[run.dir][caseID(r.testcase)]
		}
	}

	return errors.Join(w.layout().writeZoom(ctx, w.past)...)
}

// lastRunOf is the last of the runs, oldest first, that has the testcase name.
func lastRunOf(runs []pastRun, name string) (pastRun, bool) {
	for i := len(runs) - 1; i >= 0; i-- {
		if _, ok := runs[i].cases[name]; ok {
			return runs[i], true
		}
	}

	return pastRun{}, false
}

// takeRun puts on the row the testcase as the earlier run tells it: its verdict and code, the outcome of its protocol —
// not-run for a testcase that did not end —, and its steps marked by the share of the run — the steps ok first, then the failed, then the not run, the order a
// run marks them in.
func takeRun(r *row, c caseRun) {
	r.view.Mark, r.view.Code = c.mark, c.code
	r.state = stateNotRun

	if c.mark == venom.MarkPassed || c.mark == venom.MarkFailed {
		r.state = stateDone
	}

	switch {
	case r.state != stateDone:
		r.outcome = venom.ProtocolOutcomeNotRun

	case c.ok == 100:
		r.outcome = venom.ProtocolOutcomeOK

	case c.ok > 0:
		r.outcome = venom.ProtocolOutcomePartial

	default:
		r.outcome = venom.ProtocolOutcomeFailed
	}

	n := len(r.view.Steps)
	ok, failed := (c.ok*n+50)/100, (c.failed*n+50)/100

	for i := range r.view.Steps {
		mark := venom.MarkNotRun

		switch {
		case i < ok:
			mark = venom.MarkPassed

		case i < ok+failed:
			mark = venom.MarkFailed
		}

		r.view.Steps[i].Mark = mark
	}
}

// pointsOfRun are the entries and exits of the processes of the run folder dir by testcase row id, as its 01-system.md
// tells them; the failed spans of a point go to its first testcase.
func pointsOfRun(dir string) map[string][]tempo.Point {
	points := map[string][]tempo.Point{}

	body, err := os.ReadFile(filepath.Join(dir, SchemesDir, ZoomSystemFile))
	if err != nil {
		return points
	}

	for _, cells := range processRows(string(body)) {
		if len(cells) < 8 {
			continue
		}

		p := tempo.Point{Service: unescape(cells[0]), Dir: cells[1], Name: unescape(cells[2])}
		spans, _ := strconv.Atoi(cells[6])

		for i, id := range strings.Fields(unescape(cells[7])) {
			if i > 0 || spans == 0 {
				points[id] = append(points[id], p)
				continue
			}

			for range spans {
				failed := p
				failed.Failed = true
				points[id] = append(points[id], failed)
			}
		}
	}

	return points
}
