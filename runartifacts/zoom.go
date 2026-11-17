package runartifacts

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"

	"github.com/ovh/venom"
)

// The zoom set of a run folder, schemes/ top-down: the run, the system, each path, each testsuite.
const (
	ZoomRunFile    = "00-run.md"
	ZoomSystemFile = "01-system.md"
	ZoomPathsDir   = "02-paths"
	ZoomSuitesDir  = "03-suites"
)

// caseFilesFrom is the testcases of a testsuite from which each testcase of it has a zoom file of its own: a large
// testsuite is split by testcase, a small one keeps its testcases in its own file.
const caseFilesFrom = 8

// writeZoom writes the zoom set of the run as it stands, every series with every earlier run of the same set, secrets
// hidden: 00-run.md, 01-system.md, 02-paths/<path>.md, 03-suites/<testsuite>.md, a testsuite of caseFilesFrom
// testcases and more split into 03-suites/<testsuite>/<NN>-<testcase>.md, and an INDEX.md on every level.
func (l layout) writeZoom(ctx context.Context, a *archive) []error {
	hist := func(file string) []map[string]count {
		return a.pointsIn(relOf(l.dir, file))
	}

	files := map[string]string{
		l.zoomRun():                l.zoomRunText(a.runs),
		l.zoomSystem():             l.zoomSystemText(hist(l.zoomSystem())),
		l.zoomIndex():              l.zoomIndexText(),
		l.zoomLevel(ZoomPathsDir):  l.zoomPathsIndexText(),
		l.zoomLevel(ZoomSuitesDir): l.zoomSuitesIndexText(),
	}

	for _, folder := range l.order {
		file := l.zoomPath(l.names[folder])
		files[file] = l.zoomPathText(folder, a.runs, hist(file))
	}

	for _, s := range suitesOf(l.rows) {
		file := l.zoomSuite(s[0])
		files[file] = l.zoomSuiteText(s, a.runs, hist(file))

		if !split(s) {
			continue
		}

		files[filepath.Join(l.zoomCaseDir(s[0]), IndexFile)] = l.zoomCasesIndexText(s)

		for i, r := range s {
			files[l.zoomCaseFile(r, i)] = l.zoomCaseText(r, i)
		}
	}

	var errs []error

	for file, text := range files {
		errs = append(errs, writeFile(file, []byte(venom.HideSensitive(ctx, text))))
	}

	return errs
}

// bar is the progress bar of a block with its verdict: [██████████████████░░] 91 %  ❌ 80/88.
func bar(c count) string {
	return progress(c) + "  " + c.mark()
}

func (l layout) zoomDir() string {
	return filepath.Join(l.dir, SchemesDir)
}

func (l layout) zoomRun() string {
	return filepath.Join(l.zoomDir(), ZoomRunFile)
}

func (l layout) zoomSystem() string {
	return filepath.Join(l.zoomDir(), ZoomSystemFile)
}

func (l layout) zoomIndex() string {
	return filepath.Join(l.zoomDir(), IndexFile)
}

func (l layout) zoomPath(name string) string {
	return filepath.Join(l.zoomDir(), ZoomPathsDir, name+".md")
}

func (l layout) zoomSuite(r row) string {
	return filepath.Join(l.zoomDir(), ZoomSuitesDir, slug.Make(r.suiteShort)+".md")
}

// zoomLevel is the INDEX.md of the level folder of the zoom set: 02-paths or 03-suites.
func (l layout) zoomLevel(dir string) string {
	return filepath.Join(l.zoomDir(), dir, IndexFile)
}

// zoomCaseDir is the folder of the testcase files of a testsuite split by testcase.
func (l layout) zoomCaseDir(r row) string {
	return filepath.Join(l.zoomDir(), ZoomSuitesDir, slug.Make(r.suiteShort))
}

// zoomCaseFile is the file of testcase i of a testsuite split by testcase: <NN>-<testcase>.md.
func (l layout) zoomCaseFile(r row, i int) string {
	return filepath.Join(l.zoomCaseDir(r), fmt.Sprintf("%02d-%s.md", i+1, slug.Make(r.testcase)))
}

// split: the testsuite of the rows has a file per testcase.
func split(rows []row) bool {
	return len(rows) >= caseFilesFrom
}

// title is the top of the zoom set: «run <YYYY-MM>/<DD-n>» of a run, «scheme» of a scheme with no run.
func (l layout) title() string {
	if l.plan {
		return "scheme"
	}

	return "run " + l.thisRun()
}

// runLinks is the line of the files only a run writes — the report, the problems, the protocols — none in a scheme with
// no run.
func (l layout) runLinks(b *strings.Builder, base string) {
	if l.plan {
		return
	}

	fmt.Fprintf(b, "- run: %s · %s · %s\n", link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()),
		link(IndexFile, base, l.index()))
}

// upper is s with its first letter upper case.
func upper(s string) string {
	if s == "" {
		return s
	}

	return strings.ToUpper(s[:1]) + s[1:]
}

// fence is a plain code block of the lines.
func fence(b *strings.Builder, lines []string) {
	b.WriteString("```\n" + strings.Join(lines, "\n") + "\n```\n\n")
}
