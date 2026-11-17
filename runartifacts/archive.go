package runartifacts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// archive are the earlier runs of the set and, by file of the zoom set, the points of the processes each one drew.
type archive struct {
	runs   []pastRun
	points map[string][]map[string]count
}

// okPct is the ok share after a bar: [██░░] 91 %.
var okPct = regexp.MustCompile(`\] (\d+) %`)

// pointsIn are the points of the processes in the zoom file rel of every earlier run, oldest first; a run without the
// file has none. They are read once per file.
func (a *archive) pointsIn(rel string) []map[string]count {
	if hist, ok := a.points[rel]; ok {
		return hist
	}

	hist := make([]map[string]count, 0, len(a.runs))

	for _, run := range a.runs {
		body, err := os.ReadFile(filepath.Join(run.dir, rel))
		if err != nil {
			hist = append(hist, nil)
			continue
		}

		hist = append(hist, readPoints(string(body)))
	}

	a.points[rel] = hist

	return hist
}

// readPoints are the rows of the processes table of a zoom file by process, direction and point.
func readPoints(body string) map[string]count {
	points := map[string]count{}
	in := false

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			in = strings.HasPrefix(line, headProcesses)

		case in && strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| -- "):
			cells := splitCells(line)

			if len(cells) < 6 || !strings.Contains(cells[4], "/") {
				continue
			}

			c := countFrom(cells[4])

			if m := okPct.FindStringSubmatch(cells[5]); m != nil {
				c.ok = partOf(m[1], c.all)
			}

			points[unescape(cells[0])+"\x00"+cells[1]+"\x00"+unescape(cells[2])] = c

		default:
		}
	}

	return points
}

// relOf is a file of the run folder dir relative to it, the key of the same file in the earlier runs.
func relOf(dir, file string) string {
	rel, err := filepath.Rel(dir, file)
	if err != nil {
		return file
	}

	return rel
}

