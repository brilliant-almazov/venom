package runartifacts

import (
	"os"
	"sort"
	"strings"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// process is a binary of the system as its traces name it — a server, a worker, a consumer, a cron — with its entries
// and exits and the testcases that went through each.
type process struct {
	name   string
	rows   []row
	points []*endpoint
	seen   map[string]bool
}

// endpoint is an entry or an exit of a process with the testcases that cover it and the failed spans on it.
type endpoint struct {
	dir, name string
	rows      []row
	failed    int
}

// key is the process, the direction and the point, the key of its history.
func (p *endpoint) key(service string) string {
	return service + "\x00" + p.dir + "\x00" + p.name
}

// pointsOfView are the points of every saved trace of a testcase: the Tempo JSON next to each span tree.
func pointsOfView(view venom.ProtocolView) []tempo.Point {
	var points []tempo.Point
	ids := make([]string, 0, len(view.Trees))

	for id := range view.Trees {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		body, err := os.ReadFile(strings.TrimSuffix(view.Trees[id], ".md") + ".json")
		if err != nil {
			continue
		}

		points = append(points, tempo.Points(body)...)
	}

	return points
}

// processesOf are the processes the rows went through, in the order met, their points in the order met, entries first.
func processesOf(rows []row) []*process {
	var procs []*process
	at := map[string]*process{}
	pointAt := map[string]*endpoint{}

	for _, r := range rows {
		for _, p := range r.points {
			pr, ok := at[p.Service]

			if !ok {
				pr = &process{name: orNone(p.Service), seen: map[string]bool{}}
				at[p.Service] = pr
				procs = append(procs, pr)
			}

			if !pr.seen[r.testcase] {
				pr.seen[r.testcase] = true
				pr.rows = append(pr.rows, r)
			}

			k := p.Service + "\x00" + p.Dir + "\x00" + p.Name
			pt, ok := pointAt[k]

			if !ok {
				pt = &endpoint{dir: p.Dir, name: p.Name}
				pointAt[k] = pt
				pr.points = append(pr.points, pt)
			}

			if len(pt.rows) == 0 || pt.rows[len(pt.rows)-1].testcase != r.testcase {
				pt.rows = append(pt.rows, r)
			}

			if p.Failed {
				pt.failed++
			}
		}
	}

	for _, pr := range procs {
		sort.SliceStable(pr.points, func(i, j int) bool { return dirOrder[pr.points[i].dir] < dirOrder[pr.points[j].dir] })
	}

	return procs
}

// dirOrder puts the entries first, then the jobs, then the exits.
var dirOrder = map[string]int{tempo.PointIn: 0, tempo.PointRun: 1, tempo.PointOut: 2}
