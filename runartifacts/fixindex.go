package runartifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFixes redraws the fixes folder: fixes/INDEX.md with every service and its count of fixes, the INDEX.md of every
// service and a file per fix.
func (l layout) writeFixes() error {
	if err := os.RemoveAll(filepath.Join(l.dir, FixesDir)); err != nil {
		return err
	}

	base := dirOf(l.fixesIndex())
	var b strings.Builder
	b.WriteString("# Fixes\n\n")
	fmt.Fprintf(&b, "%s · %s\n\n| service | fixes | testcases |\n| -- | -- | -- |\n", link(ReportFile, base, l.report()),
		link(ProblemsFile, base, l.problems()))

	for _, s := range l.services {
		n := 0

		for _, f := range s.fixes {
			n += len(f.rows)
		}

		fmt.Fprintf(&b, "| %s | %d | %d |\n", link(cell(s.name), base, filepath.Join(s.dir, IndexFile)), len(s.fixes), n)

		if err := l.writeService(s); err != nil {
			return err
		}
	}

	if len(l.services) == 0 {
		b.WriteString("| no fixes | | |\n")
	}

	return writeFile(l.fixesIndex(), []byte(b.String()))
}

// writeService writes fixes/<service>/INDEX.md, one row per fix, and the file of every fix.
func (l layout) writeService(s fixService) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Fixes of %s\n\n%s\n\n| fix | testcases | step | reason |\n| -- | -- | -- | -- |\n", s.name,
		link(FixesDir+"/"+IndexFile, s.dir, l.fixesIndex()))

	for _, f := range s.fixes {
		var cases []string

		for _, r := range f.rows {
			cases = append(cases, cell(r.testcase))
		}

		step, _ := problemStep(f.rows[0])
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", link(filepath.Base(f.file), s.dir, f.file), strings.Join(cases, "; "), step,
			orNone(f.reason))

		if err := l.writeFix(s, f); err != nil {
			return err
		}
	}

	return writeFile(filepath.Join(s.dir, IndexFile), []byte(b.String()))
}
