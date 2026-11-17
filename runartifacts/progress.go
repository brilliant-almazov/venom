package runartifacts

import (
	"fmt"
	"strings"
)

// barCells is the width of a progress bar.
const barCells = 20

// barGlyphs are the cells of ok, partial and failed, in that order.
var barGlyphs = []string{"█", "▒", "░"}

// barNoRun is the bar of a block no run has: a scheme drawn with no run, of testcases no earlier run of the set has.
const barNoRun = "no run"

// progress is the progress bar of a block: 20 cells of ok, partial and failed shared by the largest remainder method,
// a share that is not zero has a cell at least, then the ok share rounded half up: [████████████████▒▒░░] 80 %.
func progress(c count) string {
	if c.all > 0 && c.unrun == c.all {
		return barNoRun
	}

	var b strings.Builder

	for i, n := range cellsOf([]int{c.ok, c.partial, c.notOK}, c.all) {
		b.WriteString(strings.Repeat(barGlyphs[i], n))
	}

	cells := b.String()

	if c.all == 0 {
		cells = strings.Repeat(barGlyphs[2], barCells)
	}

	return fmt.Sprintf("[%s] %d %%", cells, okShare(c))
}

// cellsOf shares barCells among the parts of all by the largest remainder method, ties to the earlier part; a part
// that is not zero and got no cell takes one from the part with the most cells.
func cellsOf(parts []int, all int) []int {
	cells := make([]int, len(parts))

	if all == 0 {
		return cells
	}

	left := barCells
	rest := make([]int, len(parts))

	for i, n := range parts {
		cells[i] = n * barCells / all
		rest[i] = n * barCells % all
		left -= cells[i]
	}

	for ; left > 0; left-- {
		top := 0

		for i := range rest {
			if rest[i] > rest[top] {
				top = i
			}
		}

		cells[top]++
		rest[top] = -1
	}

	for i, n := range parts {
		if n > 0 && cells[i] == 0 {
			cells[largest(cells)]--
			cells[i]++
		}
	}

	return cells
}

// largest is the part with the most cells, the earlier on a tie.
func largest(cells []int) int {
	top := 0

	for i := range cells {
		if cells[i] > cells[top] {
			top = i
		}
	}

	return top
}

// okShare is the share of ok in whole %, rounded half up, 0 of none.
func okShare(c count) int {
	if c.all == 0 {
		return 0
	}

	return (c.ok*200 + c.all) / (2 * c.all)
}

// stepCount is the count of the steps of a testcase, the denominator of its bar: ok passed, partial not run, failed.
func stepCount(r row) count {
	var c count

	for _, s := range r.view.Steps {
		c.add(s.Mark)
	}

	return c
}
