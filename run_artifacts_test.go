package venom

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// viewRecorder is a run artifacts writer that keeps the view of every testcase.
type viewRecorder struct {
	mu     sync.Mutex
	before int
	views  map[string]ProtocolView
}

func (r *viewRecorder) Start(context.Context, *TestSuite, *TestCase) {}

func (r *viewRecorder) BeforeVerdict(context.Context, *TestSuite, *TestCase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.before++
}

func (r *viewRecorder) Testcase(ctx context.Context, ts *TestSuite, tc *TestCase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views[CaseName(tc)] = ProtocolOf(ctx, ts, tc)
}

func (r *viewRecorder) Finish(context.Context, *Tests) error {
	return nil
}

// dbExecutor answers as an sql read: the rows of the last query in out.
type dbExecutor struct{}

func (dbExecutor) Run(context.Context, TestStep) (interface{}, error) {
	type Result struct {
		Out string `json:"out"`
	}

	return Result{Out: "7|active"}, nil
}

// monolithExecutor answers as a monolith call: a code and a body, no trace.
type monolithExecutor struct{}

func (monolithExecutor) Run(context.Context, TestStep) (interface{}, error) {
	type Result struct {
		StatusCode int    `json:"statuscode"`
		Body       string `json:"body"`
	}

	return Result{StatusCode: 200, Body: `{"ok":true}`}, nil
}

// httpCallLib is a user executor that checks the answer inside itself, as the http_call of runs 17-1…17-3.
const httpCallLib = `executor: http_call
input:
  code: 200
steps:
- type: gateway
  code: '{{.input.code}}'
  info:
  - "hop: gateway"
  assertions:
  - result.statuscode ShouldEqual 200
output:
  code: '{{.result.statuscode}}'
`

func runArtifactsSuite(t *testing.T, suite string) (*viewRecorder, string) {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".git/HEAD":               "ref: refs/heads/master\n",
		"cases/lib/http_call.yml": httpCallLib,
		"cases/a/suite.yml":       suite,
	})
	v := newTestVenom(t)
	v.RegisterExecutorBuiltin("gateway", gatewayExecutor{})
	v.RegisterExecutorBuiltin("sql", dbExecutor{})
	v.RegisterExecutorBuiltin("monolith", monolithExecutor{})
	rec := &viewRecorder{views: map[string]ProtocolView{}}
	v.Artifacts = rec
	v.OutputDir = filepath.Join(root, "raw")
	v.ProtocolDir = filepath.Join(root, "protocols")
	file := filepath.Join(root, "cases/a/suite.yml")
	ctx := context.Background()
	require.NoError(t, v.Parse(ctx, []string{file}))
	_ = v.Process(ctx, []string{file})
	return rec, v.ProtocolDir
}

// A user executor whose own check fails on a 409 still leaves the call on the protocol: the request as sent, the
// answer, the trace id and «trace not saved» with it (runs 17-1…17-3 lost them).
func TestUserExecutorCallKeptOnFailure(t *testing.T) {
	rec, dir := runArtifactsSuite(t, `name: suite
testcases:
- name: S-001 conflict
  steps:
  - type: http_call
    code: 409
`)
	view := rec.views["S-001 conflict"]
	assert.Equal(t, 1, rec.before)
	assert.Equal(t, MarkFailed, view.Mark)
	assert.Equal(t, "409", view.Code)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", view.TraceID)
	assert.Empty(t, view.TraceFile)
	assert.Equal(t, HopView{Mark: MarkFailed, Code: "409", TraceID: "0af7651916cd43dd8448eb211c80319c", Reason: view.Hops["gateway"].Reason}, view.Hops["gateway"])
	assert.NotEmpty(t, view.Hops["gateway"].Reason)
	for _, want := range []string{"- method: POST", "- url: http://nginx-gateway/v1/billing/accounts", `"tenant_id": "7"`, "- statuscode: 409",
		protocolNoTree + " 0af7651916cd43dd8448eb211c80319c"} {
		assert.Contains(t, view.Text, want)
	}
	assert.Contains(t, readProtocol(t, dir, "s-001-conflict"), "- statuscode: 409")
}

// A monolith call returns no trace: the database reads of the testcase stand in the place of the span tree.
func TestProtocolDBReadsInPlaceOfTrace(t *testing.T) {
	rec, _ := runArtifactsSuite(t, `name: suite
testcases:
- name: M-001 monolith
  steps:
  - type: monolith
    assertions:
    - result.statuscode ShouldEqual 200
  - type: sql
    commands:
    - "SELECT id, state FROM accounts WHERE id = 7"
`)
	view := rec.views["M-001 monolith"]
	assert.Equal(t, MarkPassed, view.Mark)
	for _, want := range []string{"- db read: step 2", "  - query: `SELECT id, state FROM accounts WHERE id = 7`", "7|active"} {
		assert.Contains(t, view.Text, want)
	}
}
