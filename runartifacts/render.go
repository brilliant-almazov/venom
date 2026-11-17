package runartifacts

import (
	"context"
	"path/filepath"

	"github.com/ovh/venom"
)

// refresh redraws what one testcase is on: the index, the fixes, the report, the problems, the scheme of the system, of its path
// and of the case. A run killed after it leaves them as they stand.
func (w *Writer) refresh(ctx context.Context, r *row) {
	l := w.layout()
	name := l.pathOf(*r)

	for _, err := range []error{
		l.writeIndex(), l.writeFixes(), l.writeReport(w), l.writeProblems(w), l.writeSystem(), l.writePath(name),
		l.writeCase(*r),
	} {
		if err != nil {
			venom.Error(ctx, "unable to write the run folder %s: %v", w.dir, err)
		}
	}
}

// render redraws the whole run folder: the index, the fixes, the report, the problems and every scheme.
func (w *Writer) render(ctx context.Context) error {
	l := w.layout()
	errs := []error{l.writeIndex(), l.writeFixes(), l.writeReport(w), l.writeProblems(w), l.writeSystem()}

	for _, folder := range l.order {
		errs = append(errs, l.writePath(l.names[folder]))
	}

	for _, r := range l.rows {
		errs = append(errs, l.writeCase(r))
	}

	for _, err := range errs {
		if err != nil {
			venom.Error(ctx, "unable to write the run folder %s: %v", w.dir, err)
			return err
		}
	}

	return nil
}

func dirOf(file string) string {
	return filepath.Dir(file)
}
