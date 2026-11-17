package runartifacts

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ovh/venom"
)

// Testcase writes protocols/<testsuite>/<row id>-<ok|partial|failed>-<rest>.md the moment the testcase ends, copies its span trees to
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
	w.renameProtocol(r, venom.ProtocolOutcomeFile(tc))
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

// renameProtocol names the protocol of r by the outcome of its testcase; the protocol written under the former name as
// the testcase began is removed, so one file per testcase stays.
func (w *Writer) renameProtocol(r *row, name string) {
	p := filepath.Join(filepath.Dir(r.protocol), name+".md")
	if p == r.protocol {
		return
	}

	if err := os.Remove(r.protocol); err != nil && !os.IsNotExist(err) {
		venom.Error(context.Background(), "unable to remove protocol %s: %v", r.protocol, err)
	}

	r.protocol = p
}
