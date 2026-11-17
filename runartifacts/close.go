package runartifacts

import (
	"context"
	"errors"
	"time"

	"github.com/ovh/venom"
)

// Finish writes the protocol of every testcase the run never started, redraws the whole run folder and removes every
// empty folder of it.
func (w *Writer) Finish(ctx context.Context, _ *venom.Tests) error {
	return w.close(ctx, "", reasonNotStarted)
}

// Abort closes a run that ended early — interrupted by a signal, stopped by an error of its setup, crashed — with
// reason: the testcase under way stays interrupted, every testcase not started gets a protocol «not run» with reason.
func (w *Writer) Abort(ctx context.Context, reason string) error {
	return w.close(ctx, reason, reason)
}

func (w *Writer) close(ctx context.Context, runErr, notStarted string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true
	w.runErr = runErr
	w.end = time.Now()

	for _, r := range w.rows {
		switch r.state {
		case statePending:
			r.state = stateNotRun
			r.view.Reason = stateNotRun + ": " + notStarted
			w.writeStub(ctx, r)

		case stateInterrupted:
			if runErr != "" {
				r.view.Reason = stateInterrupted + ": " + runErr
				w.writeStub(ctx, r)
			}

		default:
		}
	}

	err := w.render(ctx)
	if err == nil {
		err = w.fillSchemes(ctx)
	}

	if err == nil {
		err = errors.Join(w.layout().writeZoom(ctx, w.history())...)
	}

	removeEmptyDirs(w.dir)

	return err
}
