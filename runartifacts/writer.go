// Package runartifacts lays a run out in one folder (--run-dir): protocols with a protocol per testcase in the owner's
// form, the span trees and INDEX.md; schemes, the scheme templates of the testsuite set filled with the run; raw, every
// other output of Venom.
package runartifacts

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gosimple/slug"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// The folders of a run, lower case.
const (
	ProtocolsDir = "protocols"
	TracesDir    = "traces"
	SchemesDir   = "schemes"
	RawDir       = "raw"
	IndexFile    = "INDEX.md"
)

// fetchWait is how long a span tree missing from the trace folders is waited for in Tempo.
const fetchWait = 10 * time.Second

// Writer is the run artifacts writer of one run folder.
type Writer struct {
	dir  string
	mu   sync.Mutex
	rows []row
}

// row is one testcase of the run as the index and the schemes read it.
type row struct {
	suite, suiteFile, testcase string
	index                      int
	protocol, trace            string
	view                       venom.ProtocolView
	schemes                    string
}

// New is the writer of the run folder dir.
func New(dir string) *Writer {
	return &Writer{dir: dir}
}

// RawPath is the folder of the raw output of Venom: reports, venom.log, dumps, io.jsonl.
func (w *Writer) RawPath() string {
	return filepath.Join(w.dir, RawDir)
}

// BeforeVerdict fetches from Tempo every span tree of a traced call that no step saved, so the trace gate and the
// protocol see it; a trace that Tempo does not have stays missing and the protocol says «trace not saved».
func (w *Writer) BeforeVerdict(ctx context.Context, _ *venom.TestSuite, tc *venom.TestCase) {
	ids := venom.TracesWithoutTree(tc)
	if len(ids) == 0 {
		return
	}

	dir := filepath.Join(w.RawPath(), TracesDir)
	tc.AddTraceDir(dir)
	url := venom.TempoURL(tc)

	if url == "" {
		return
	}

	for _, id := range ids {
		tempo.Save(ctx, url, dir, id, fetchWait)
	}
}

// Testcase writes protocols/<testsuite>/<testcase>.md and copies the span trees of the testcase to protocols/traces.
func (w *Writer) Testcase(ctx context.Context, ts *venom.TestSuite, tc *venom.TestCase) {
	view := venom.ProtocolOf(ctx, ts, tc)
	protocol := filepath.Join(w.dir, ProtocolsDir, slug.Make(ts.ShortName), venom.ProtocolFileName(view.Name)+".md")

	if err := writeFile(protocol, []byte(view.Text)); err != nil {
		venom.Error(ctx, "unable to write protocol %s: %v", protocol, err)
	}

	trace := ""

	for id, f := range view.Trees {
		to := filepath.Join(w.dir, ProtocolsDir, TracesDir, id+".md")

		if err := copyHidden(ctx, f, to); err != nil {
			venom.Error(ctx, "unable to copy span tree %s: %v", f, err)
			continue
		}

		if id == view.TraceID {
			trace = to
		}
	}

	view.Text = ""
	r := row{suite: ts.Name, suiteFile: ts.Filepath, testcase: view.Name, index: caseIndex(ts, tc), protocol: protocol,
		trace: trace, view: view, schemes: findSchemes(ts.Filepath)}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.rows = append(w.rows, r)
}

// Finish writes protocols/INDEX.md and fills the schemes of the run.
func (w *Writer) Finish(ctx context.Context, _ *venom.Tests) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	sortRows(w.rows)

	if err := w.writeIndex(); err != nil {
		return err
	}

	return w.fillSchemes(ctx)
}

func caseIndex(ts *venom.TestSuite, tc *venom.TestCase) int {
	for i := range ts.TestCases {
		if &ts.TestCases[i] == tc {
			return i
		}
	}

	return len(ts.TestCases)
}

func writeFile(file string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}

	return os.WriteFile(file, body, 0o644)
}

// copyHidden copies a span tree with the secrets of the run hidden, as the protocol that prints it.
func copyHidden(ctx context.Context, from, to string) error {
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}

	return writeFile(to, []byte(venom.HideSensitive(ctx, string(body))))
}
