package runartifacts

import (
	"context"
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

// writeZoom writes the zoom set of the run as it stands, every series with every earlier run of the same set, secrets
// hidden: 00-run.md, 01-system.md, 02-paths/<path>.md, 03-suites/<testsuite>.md and schemes/INDEX.md.
func (l layout) writeZoom(ctx context.Context, a *archive) []error {
	hist := func(file string) []map[string]count {
		return a.pointsIn(relOf(l.dir, file))
	}

	files := map[string]string{
		l.zoomRun():    l.zoomRunText(a.runs),
		l.zoomSystem(): l.zoomSystemText(hist(l.zoomSystem())),
		l.zoomIndex():  l.zoomIndexText(),
	}

	for _, folder := range l.order {
		file := l.zoomPath(l.names[folder])
		files[file] = l.zoomPathText(folder, a.runs, hist(file))
	}

	for _, s := range suitesOf(l.rows) {
		file := l.zoomSuite(s[0])
		files[file] = l.zoomSuiteText(s, a.runs, hist(file))
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

// fence is a plain code block of the lines.
func fence(b *strings.Builder, lines []string) {
	b.WriteString("```\n" + strings.Join(lines, "\n") + "\n```\n\n")
}
