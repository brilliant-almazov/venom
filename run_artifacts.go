package venom

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// RunArtifacts lays the artifacts of a run out in a folder of its own (package runartifacts, --run-dir): it sees each
// testcase as it starts, so a run killed under it leaves the testcase marked interrupted; before its trace gate, so a
// span tree missing from the trace folders can be fetched from Tempo; then once its status is known, whatever the
// outcome; and the run once it ends.
type RunArtifacts interface {
	Start(ctx context.Context, ts *TestSuite, tc *TestCase)
	BeforeVerdict(ctx context.Context, ts *TestSuite, tc *TestCase)
	Testcase(ctx context.Context, ts *TestSuite, tc *TestCase)
	Finish(ctx context.Context, tests *Tests) error
}

// Marks of a testcase or a hop: passed, failed, not run (skipped, or never reached).
const (
	MarkPassed = "✅"
	MarkFailed = "❌"
	MarkNotRun = "⏸"
)

// ProtocolView is what a run artifacts writer takes from one testcase: the protocol text and the facts of its call,
// its steps and its hops.
type ProtocolView struct {
	// Name is the testcase name as written, row id first.
	Name string
	// Suite is the testsuite name, File its file as given to the run, Line the line of the testcase in it.
	Suite   string
	File    string
	Line    int
	Text    string
	Mark    string
	Code    string
	TraceID string
	// TraceFile is the saved span tree <dir>/<trace_id>.md of the call, empty when none was saved.
	TraceFile string
	// Trees are the saved span trees of every traced step by trace id, the call's among them.
	Trees  map[string]string
	Reason string
	// Hops are the steps with an info line "hop: <key>" by key.
	Hops map[string]HopView
	// Request is the call line of the call of the testcase, CallStep its 1-based step, 0 with no call.
	Request  string
	CallStep int
	// Expected are the info lines happens, expect, right and norm as "key: value".
	Expected []string
	// Steps are the transitions of the testcase in file order: every step of the scenario, run or not.
	Steps []StepView
}

// StepView is one step of a testcase, a transition of its scheme.
type StepView struct {
	// Number is the 1-based step of the testcase, Line its line in the testsuite file, 0 when unknown.
	Number  int
	Line    int
	Name    string
	Type    string
	Request string
	Mark    string
	Code    string
	TraceID string
	Reason  string
	// Body is the body the step answered, secrets hidden, empty when it answered none.
	Body string
	StepFacts
}

// HopView is one hop of a testcase: the step that carries "hop: <key>".
type HopView struct {
	Mark, Code, TraceID, Reason string
}

// AddTraceDir adds a folder the span trees of the testcase are looked up in, next to the trace_dir and traces of its steps.
func (tc *TestCase) AddTraceDir(dir string) {
	tc.traceDirs = append(tc.traceDirs, dir)
}

// TracesWithoutTree are the trace ids of the traced calls of the testcase whose span tree is in no trace folder.
func TracesWithoutTree(tc *TestCase) []string {
	var ids []string
	seen := map[string]bool{}
	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		id := protocolTraceID(r)
		if id == "" || !protocolHasCode(r) || seen[id] {
			continue
		}
		seen[id] = true
		if f, _ := protocolTreeFile(tc, id); f == "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// tempoKeys name the Tempo address: tempo_url of the native executors, tempo of the user executors (http_call,
// trace_save) and of the suite var (run 17-7: every step of the case said tempo, the trace was never fetched).
var tempoKeys = []string{"tempo_url", "tempo"}

// TempoURL is the tempo_url or tempo of the first step that has one, else the var tempo_url or tempo of the testcase.
func TempoURL(tc *TestCase) string {
	for i := range tc.TestStepResults {
		m := protocolStep(&tc.TestStepResults[i])
		for _, k := range tempoKeys {
			if u := protocolField(m[k]); u != "" {
				return u
			}
		}
	}
	for _, k := range tempoKeys {
		if u, ok := tc.Vars[k]; ok && u != nil && fmt.Sprint(u) != "" {
			return fmt.Sprint(u)
		}
	}
	return ""
}

// ProtocolBodyBlock prints a body as the protocol does: verbatim in a code block, a JSON body one key per line, cut.
func ProtocolBodyBlock(title, body string) string {
	return protocolBodyBlock(title, body)
}

// ProtocolFileName is the file name of the protocol of a testcase named name, with no extension.
func ProtocolFileName(name string) string {
	return protocolFileName(name)
}

// protocolRowID is the row id a testcase name starts with: SB-01-01, SB-02-01-03, GW-B0005, S-001.
var protocolRowID = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z]?[0-9][0-9A-Z]*)+`)

// ProtocolOutcome is the outcome ok, partial or failed of the testcase (section 10), never empty.
func ProtocolOutcome(tc *TestCase) string {
	return protocolOutcome(tc)
}

// ProtocolOutcomeFile is the file name of the protocol of a testcase that ended, with no extension: its outcome ok,
// partial or failed (section 10) in the name by ProtocolOutcomeName.
func ProtocolOutcomeFile(tc *TestCase) string {
	return ProtocolOutcomeName(CaseName(tc), protocolOutcome(tc))
}

// ProtocolOutcomeName is the file name of the protocol of the testcase named name with the outcome o, with no
// extension: o between the row id and the rest of the name, <row id>-<o>-<rest>, or first, <o>-<name>, when the name
// has no row id. Every protocol carries an outcome: a testcase that did not end is named with ProtocolOutcomeFailed.
func ProtocolOutcomeName(name, o string) string {
	n := protocolFileName(name)
	id := protocolRowID.FindString(n)
	if id == "" || id == n {
		return strings.TrimSuffix(o+"-"+n, "-")
	}
	return id + "-" + o + strings.TrimPrefix(n, id)
}

// CaseName is the testcase name as written in the testsuite; Venom runs it under its slug.
func CaseName(tc *TestCase) string {
	if tc.originalName != "" {
		return tc.originalName
	}

	return tc.Name
}
