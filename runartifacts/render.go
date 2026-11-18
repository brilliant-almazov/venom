package runartifacts

import (
	"context"
	"path/filepath"

	"github.com/ovh/venom"
)

// refresh redraws what one testcase is on: the index, the fixes, the report, the problems, then the schemes — the copy
// of the schemes drawn at intake with their anchors and progress bars filled (fillSchemes), or with none drawn at intake
// the scheme of the system, of its path and of the case and the zoom set. A run killed after it leaves them as they
// stand.
func (w *Writer) refresh(ctx context.Context, r *row) {
	l := w.layout()
	errs := []error{l.writeIndex(), l.writeFixes(), l.writeReport(w), l.writeProblems()}

	if w.intake() {
		errs = append(errs, w.fillSchemes(ctx))
	} else {
		errs = append(errs, l.writeSystem(), l.writePath(l.pathOf(*r)), l.writeCase(*r))
		errs = append(errs, l.writeZoom(ctx, w.history())...)
	}

	for _, err := range errs {
		if err != nil {
			venom.Error(ctx, "unable to write the run folder %s: %v", w.dir, err)
		}
	}
}

// render redraws the whole run folder: the index, the fixes, the report, the problems, then the schemes filled from
// those drawn at intake, or with none every scheme drawn from the run and the zoom set.
func (w *Writer) render(ctx context.Context) error {
	l := w.layout()
	errs := []error{l.writeIndex(), l.writeFixes(), l.writeReport(w), l.writeProblems()}

	if w.intake() {
		errs = append(errs, w.fillSchemes(ctx))
	} else {
		errs = append(errs, l.writeSystem())

		for _, folder := range l.order {
			errs = append(errs, l.writePath(l.names[folder]))
		}

		for _, r := range l.rows {
			errs = append(errs, l.writeCase(r))
		}

		errs = append(errs, l.writeZoom(ctx, w.history())...)
	}

	for _, err := range errs {
		if err != nil {
			venom.Error(ctx, "unable to write the run folder %s: %v", w.dir, err)
			return err
		}
	}

	return nil
}

// intake: the set has schemes drawn at intake (schemeFolders); venom then draws no scheme of its own and only fills
// those.
func (w *Writer) intake() bool {
	return len(w.schemeFolders()) > 0
}

func dirOf(file string) string {
	return filepath.Dir(file)
}
