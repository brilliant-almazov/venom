package runartifacts

import (
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"

	"github.com/ovh/venom"
)

// layout is the run folder as one render draws it: the testcases in testsuite file order and the paths they lie on, a
// path being the folder of a testsuite file (scenarios/supabase/<path>/SB-NN-NN-*.yml).
type layout struct {
	dir   string
	rows  []row
	names map[string]string
	order []string
	// services are the fixes of the run by service to fix (planFixes), fileOf the fix file of a row that did not pass.
	services []fixService
	fileOf   map[string]string
	// plan: no run made the folder, only the zoom set is in it (WritePlan).
	plan bool
	// past are the earlier runs of the set, oldest first, the previous state of every testcase (dynamics.go).
	past []pastRun
}

// layout is the run folder of the rows as they stand.
func (w *Writer) layout() layout {
	rows := make([]row, 0, len(w.rows))

	for _, r := range w.rows {
		rows = append(rows, *r)
	}

	sortRows(rows)
	l := layout{dir: w.dir, rows: rows, names: map[string]string{}, plan: w.plan, past: w.history().runs}
	bases := map[string][]string{}

	for _, r := range rows {
		folder := folderOf(r)

		if _, ok := l.names[folder]; ok {
			continue
		}

		l.names[folder] = ""
		l.order = append(l.order, folder)
		base := filepath.Base(folder)
		bases[base] = append(bases[base], folder)
	}

	for base, folders := range bases {
		for _, folder := range folders {
			l.names[folder] = pathName(base, folder, len(folders))
		}
	}

	l.planFixes()

	return l
}

// pathName is the base name of the folder, the whole folder as a slug when two paths share the base name.
func pathName(base, folder string, sharing int) string {
	if sharing > 1 || base == "." || base == string(filepath.Separator) {
		return slug.Make(folder)
	}

	return base
}

// folderOf is the folder of the testsuite file of the row, as given to the run.
func folderOf(r row) string {
	return filepath.ToSlash(filepath.Dir(r.suiteFile))
}

// pathOf is the path name of the row.
func (l layout) pathOf(r row) string {
	return l.names[folderOf(r)]
}

// rowsOf are the rows on the path of folder.
func (l layout) rowsOf(folder string) []row {
	var rows []row

	for _, r := range l.rows {
		if folderOf(r) == folder {
			rows = append(rows, r)
		}
	}

	return rows
}

func (l layout) systemScheme() string {
	return filepath.Join(l.dir, SchemesDir, SystemFile)
}

func (l layout) pathScheme(name string) string {
	return filepath.Join(l.dir, SchemesDir, name, PathFile)
}

func (l layout) caseScheme(r row) string {
	return filepath.Join(l.dir, SchemesDir, l.pathOf(r), venom.ProtocolFileName(r.testcase)+".md")
}

func (l layout) index() string {
	return filepath.Join(l.dir, ProtocolsDir, IndexFile)
}

func (l layout) report() string {
	return filepath.Join(l.dir, ReportFile)
}

func (l layout) problems() string {
	return filepath.Join(l.dir, ProblemsFile)
}

func (l layout) fixesIndex() string {
	return filepath.Join(l.dir, FixesDir, IndexFile)
}

// written: the protocol file of the row is on disk, a pending testcase has none yet.
func written(r row) bool {
	return r.state != statePending
}

// one cuts a text to one line for a table cell.
func one(s string) string {
	s = strings.Join(strings.Fields(s), " ")

	if r := []rune(s); len(r) > cellMax {
		s = string(r[:cellMax]) + "…"
	}

	return cell(s)
}

// cellMax caps the text of a table cell; the protocol holds it whole.
const cellMax = 300
