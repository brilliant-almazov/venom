package runartifacts

import (
	"fmt"
	"path/filepath"
	"strings"
)

// point is one run of a series: its name and its count.
type point struct {
	name string
	c    count
}

// seriesOf is the series of one slice across the runs: every earlier run that has it, then this run.
func seriesOf(past []pastRun, pick func(pastRun) (count, bool), name string, now count) []point {
	var points []point

	for _, p := range past {
		if c, ok := pick(p); ok {
			points = append(points, point{name: p.name, c: c})
		}
	}

	return append(points, point{name: name + " (this run)", c: now})
}

// drawSeries is the history of a series in a plain code block: per run its bar, Δ p.p. and Δ abs. of ok against
// the run before.
func drawSeries(b *strings.Builder, points []point) {
	nameW, barW := len([]rune("run")), 0
	bars := make([]string, len(points))

	for i, p := range points {
		bars[i] = bar(p.c)
		nameW = max(nameW, len([]rune(p.name)))
		barW = max(barW, len([]rune(bars[i])))
	}

	lines := []string{pad("run", nameW) + "  " + pad("bar", barW) + "  " + fmt.Sprintf("%7s  %6s", "Δ p.p.", "Δ abs.")}

	for i, p := range points {
		dpp, dabs := noValue, noValue

		if i > 0 {
			prev := points[i-1].c
			dpp = fmt.Sprintf("%+.1f", share(p.c)-share(prev))
			dabs = fmt.Sprintf("%+d", p.c.ok-prev.ok)
		}

		lines = append(lines, pad(p.name, nameW)+"  "+pad(bars[i], barW)+"  "+fmt.Sprintf("%7s  %6s", dpp, dabs))
	}

	fence(b, lines)
}

// pad fills s with spaces to width runes.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-len([]rune(s))))
}

// share is ok of all in %, 0 of none.
func share(c count) float64 {
	if c.all == 0 {
		return 0
	}

	return float64(c.ok) * 100 / float64(c.all)
}

// thisRun is the name of the run folder among the runs of its set: <YYYY-MM>/<DD-n>, the folder name out of a set.
func (l layout) thisRun() string {
	abs, err := filepath.Abs(l.dir)
	if err != nil {
		return filepath.Base(l.dir)
	}

	return runName(filepath.Dir(filepath.Dir(abs)), abs)
}

func pickTotal(p pastRun) (count, bool) {
	return p.total, p.total.all > 0
}

func pickPath(name string) func(pastRun) (count, bool) {
	return func(p pastRun) (count, bool) {
		c, ok := p.paths[name]
		return c, ok
	}
}

func pickSuite(name string) func(pastRun) (count, bool) {
	return func(p pastRun) (count, bool) {
		c, ok := p.suites[name]
		return c, ok
	}
}
