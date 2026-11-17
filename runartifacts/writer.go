// Package runartifacts lays a run out in one folder (--run-dir), with no agent and no hand: protocols with a protocol
// per testcase written the moment the testcase ends, the span trees and INDEX.md; schemes drawn from the results of the
// run — the system, each path, each case; REPORT.md and PROBLEMS.md; raw, every other output of Venom. A run killed,
// interrupted or crashed leaves the protocol of every finished testcase and the testcase under way marked interrupted.
package runartifacts

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// The folders and files of a run.
const (
	ProtocolsDir = "protocols"
	TracesDir    = "traces"
	SchemesDir   = "schemes"
	RawDir       = "raw"
	IndexFile    = "INDEX.md"
	ReportFile   = "REPORT.md"
	ProblemsFile = "PROBLEMS.md"
	SystemFile   = "SYSTEM.md"
	PathFile     = "PATH.md"
)

// The states of a testcase in the run folder.
const (
	stateDone        = "done"
	statePending     = "pending"
	stateInterrupted = "interrupted"
	stateNotRun      = "not run"
)

// The reasons of a testcase that did not end.
const (
	reasonUnderWay   = "the run ended before the testcase did"
	reasonNotStarted = "the run ended before the testcase started"
)

// fetchWait is how long a span tree missing from the trace folders is waited for in Tempo before the verdict: as long
// as the late fetch of the tempo executor, so a trace Tempo ingests late is in the protocol (run 17-12, SB-05-01: the
// tree of the monolith webhook call came after 10 s, the protocol said «trace not saved»).
var fetchWait = tempo.LateWait

// Writer is the run artifacts writer of one run folder.
type Writer struct {
	dir    string
	mu     sync.Mutex
	rows   map[string]*row
	start  time.Time
	end    time.Time
	runErr string
	closed bool
}

// row is one testcase of the run as the protocols, the index, the schemes and the report read it.
type row struct {
	suite, suiteShort, suiteFile, absFile, testcase string
	index                                           int
	protocol, trace                                 string
	// trees are the copied span trees of the testcase by trace id.
	trees   map[string]string
	view    venom.ProtocolView
	schemes string
	state   string
	// fixer is the service to fix of a testcase that ended and did not pass (fixerOf).
	fixer string
	// outcome is ok, partial or failed of the protocol, failed until the testcase ends with one of its own.
	outcome string
}

// New is the writer of the run folder dir.
func New(dir string) *Writer {
	return &Writer{dir: dir, rows: map[string]*row{}, start: time.Now()}
}

// RawPath is the folder of the raw output of Venom: reports, venom.log, dumps, io.jsonl.
func (w *Writer) RawPath() string {
	return filepath.Join(w.dir, RawDir)
}

// Plan takes every testcase of the parsed testsuites as pending, so a run that ends early still names each one.
func (w *Writer) Plan(tests *venom.Tests) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range tests.TestSuites {
		ts := &tests.TestSuites[i]

		for j := range ts.TestCases {
			r := w.rowOf(ts, j)
			r.view = venom.PlannedView(ts, &ts.TestCases[j])
			r.state = statePending
		}
	}
}

// Start writes the protocol of a testcase that begins as interrupted: a run killed under it leaves it so.
func (w *Writer) Start(ctx context.Context, ts *venom.TestSuite, tc *venom.TestCase) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	r := w.rowOf(ts, caseIndex(ts, tc))
	if r.state != statePending {
		r.view = venom.PlannedView(ts, tc)
	}

	r.state = stateInterrupted
	r.view.Reason = stateInterrupted + ": " + reasonUnderWay
	w.writeStub(ctx, r)
	w.refresh(ctx, r)
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
		venom.Error(ctx, "no Tempo address for traces %v: no step has tempo_url or tempo, nor the vars", ids)
		return
	}

	for _, id := range ids {
		tempo.Save(ctx, url, dir, id, fetchWait)
	}
}
