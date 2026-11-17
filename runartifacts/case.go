package runartifacts

import (
	"context"
	"path/filepath"

	"github.com/ovh/venom"
)

// Testcase writes protocols/<testsuite>/<testcase>.md the moment the testcase ends, copies its span trees to
// protocols/traces and redraws the index, the report, the problems and the schemes it is on.
func (w *Writer) Testcase(ctx context.Context, ts *venom.TestSuite, tc *venom.TestCase) {
	view := venom.ProtocolOf(ctx, ts, tc)

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	r := w.rowOf(ts, caseIndex(ts, tc))
	r.state = stateDone
	r.trace = ""

	for id, f := range view.Trees {
		to := filepath.Join(w.dir, ProtocolsDir, TracesDir, id+".md")

		if err := copyHidden(ctx, f, to); err != nil {
			venom.Error(ctx, "unable to copy span tree %s: %v", f, err)
			continue
		}

		r.trees[id] = to

		if id == view.TraceID {
			r.trace = to
		}
	}

	text := view.Text
	view.Text = ""
	r.view = view
	r.schemes = findSchemes(ts.Filepath)

	if err := writeFile(r.protocol, []byte(venom.HideSensitive(ctx, withLinks(text, w.links(r))))); err != nil {
		venom.Error(ctx, "unable to write protocol %s: %v", r.protocol, err)
	}

	w.refresh(ctx, r)
}
