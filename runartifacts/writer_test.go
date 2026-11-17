package runartifacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

const (
	traceOK   = "0af7651916cd43dd8448eb211c80319c"
	traceLost = "4bf92f3577b34da6a3ce929d0e0e4736"
)

// scheme is a template of the testsuite set with every form of anchor.
const scheme = `gateway ──► billing   ⟦*:billing⟧
S-001: ⟦S-001⟧ hop ⟦S-001:billing⟧ reason ⟦S-001:billing:reason⟧ ⟦S-001:protocol⟧ ⟦S-001:trace⟧
S-002: ⟦S-002⟧ hop ⟦S-002:billing⟧ reason ⟦S-002:billing:reason⟧ ⟦S-002:trace⟧
S-404: ⟦S-404⟧ ⟦S-404:billing:reason⟧
`

func call(status venom.Status, code int, trace, traceDir string, errs ...string) venom.TestStepResult {
	r := venom.TestStepResult{
		Name:   "call",
		Status: status,
		ComputedVars: venom.H{
			"result.statuscode": code,
			"result.trace_id":   trace,
		},
		Interpolated: []byte("type: http\ntrace_dir: " + traceDir + "\n"),
		ComputedInfo: []string{"hop: billing"},
	}

	for _, e := range errs {
		r.Errors = append(r.Errors, venom.Failure{Value: e})
	}

	return r
}

func TestRunFolder(t *testing.T) {
	root := t.TempDir()
	traces := filepath.Join(root, "saved")
	suiteFile := filepath.Join(root, "cases", "billing", "a", "suite.yml")
	require.NoError(t, os.MkdirAll(traces, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cases", "billing", SchemesDir, "1-system"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(traces, traceOK+".md"), []byte("svc-billing ✅"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cases", "billing", SchemesDir, "1-system", "system.md"), []byte(scheme), 0o644))

	ts := &venom.TestSuite{Name: "billing", ShortName: "billing", Filepath: suiteFile}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "S-001 create"}, Status: venom.StatusPass,
			TestStepResults: []venom.TestStepResult{call(venom.StatusPass, 200, traceOK, traces)}},
		{TestCaseInput: venom.TestCaseInput{Name: "S-002 conflict"}, Status: venom.StatusFail,
			TestStepResults: []venom.TestStepResult{call(venom.StatusFail, 409, traceLost, traces, "expected 200 got 409")}},
	}

	run := filepath.Join(root, "run")
	w := New(run)
	ctx := context.Background()

	for i := range ts.TestCases {
		w.BeforeVerdict(ctx, ts, &ts.TestCases[i])
		w.Testcase(ctx, ts, &ts.TestCases[i])
	}

	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	read := func(rel string) string {
		body, err := os.ReadFile(filepath.Join(run, rel))
		require.NoError(t, err, rel)

		return string(body)
	}

	assert.Contains(t, read("protocols/billing/S-001-ok-create.md"), "svc-billing ✅")
	assert.Contains(t, read("protocols/billing/S-002-failed-conflict.md"), "- trace not saved: "+traceLost)
	assert.Equal(t, "svc-billing ✅", read("protocols/traces/"+traceOK+".md"))

	index := read("protocols/INDEX.md")
	assert.Contains(t, index, "| billing | S-001 create | ✅ 200 | [0af76519](traces/"+traceOK+".md) | [protocol](billing/S-001-ok-create.md) |")
	assert.Contains(t, index, "| billing | S-002 conflict | ❌ 409 | trace not saved `"+traceLost+"` | [protocol](billing/S-002-failed-conflict.md) |")

	filled := read("schemes/1-system/system.md")
	assert.Equal(t, "gateway ──► billing   ❌ 1/2\n"+
		"S-001: ✅ 200 `0af76519` hop ✅ 200 `0af76519` reason — [protocol](../../protocols/billing/S-001-ok-create.md) [0af76519](../../protocols/traces/"+traceOK+".md)\n"+
		"S-002: ❌ 409 `4bf92f35` hop ❌ 409 `4bf92f35` reason expected 200 got 409 trace not saved `"+traceLost+"`\n"+
		"S-404: ⏸ not run\n", filled)
}

// The schemes live with the case set, not with the testsuites: <set>/schemes for a run folder <set>/runs/<YYYY-MM>/<DD-n>;
// a case is found by the row id at the start of its testsuite name too.
func TestSchemesOfTheCaseSet(t *testing.T) {
	root := t.TempDir()
	set := filepath.Join(root, "cases", "supabase")
	require.NoError(t, os.MkdirAll(filepath.Join(set, SchemesDir, "3-cases"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(set, SchemesDir, "3-cases", "SB-01-02.md"), []byte("⟦SB-01-02⟧ ⟦SB-01-02:monolith-db⟧"), 0o644))

	ts := &venom.TestSuite{Name: "SB-01-02 sign up", ShortName: "sign-up", Filepath: filepath.Join(root, "scenarios", "suite.yml")}
	r := call(venom.StatusPass, 200, "", "")
	r.ComputedInfo = []string{"hop: monolith-db"}
	ts.TestCases = []venom.TestCase{{TestCaseInput: venom.TestCaseInput{Name: "user row"}, Status: venom.StatusPass,
		TestStepResults: []venom.TestStepResult{r}}}

	run := filepath.Join(set, "runs", "2026-11", "17-4")
	w := New(run)
	ctx := context.Background()
	w.Testcase(ctx, ts, &ts.TestCases[0])
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	body, err := os.ReadFile(filepath.Join(run, SchemesDir, "3-cases", "SB-01-02.md"))
	require.NoError(t, err)
	assert.Equal(t, "✅ 200 ✅ 200", string(body))
}

// A call of a user executor names Tempo by tempo, not tempo_url, and its trace_save was given no traces folder: the
// span tree is fetched before the verdict and lies in the protocol and protocols/traces (run 17-7, SB-01-01).
func TestTraceFetchedByTempoOfUserExecutor(t *testing.T) {
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces/"+traceOK {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-gateway"}}]},` +
			`"scopeSpans":[{"spans":[{"traceId":"` + traceOK + `","spanId":"a","name":"/v1/supabase/webhook/{integration_hash_ids}",` +
			`"startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`))
	}))
	defer tempo.Close()

	r := call(venom.StatusPass, 200, traceOK, "")
	r.Name = "http_call"
	r.Interpolated = []byte("type: http_call\ntempo: " + tempo.URL + "\ntraces: \"\"\n")
	save := venom.TestStepResult{Name: "trace_save", Status: venom.StatusPass,
		Interpolated: []byte("type: trace_save\ntrace: " + traceOK + "\ntempo: " + tempo.URL + "\ntraces: \"\"\n")}

	ts := &venom.TestSuite{Name: "SB-01-01", ShortName: "receive-webhook-ok", Filepath: filepath.Join(t.TempDir(), "suite.yml")}
	ts.TestCases = []venom.TestCase{{TestCaseInput: venom.TestCaseInput{Name: "SB-01-01 webhook"}, Status: venom.StatusPass,
		TestStepResults: []venom.TestStepResult{r, save}}}

	run := filepath.Join(t.TempDir(), "run")
	w := New(run)
	ctx := context.Background()
	tc := &ts.TestCases[0]
	w.BeforeVerdict(ctx, ts, tc)
	assert.Empty(t, venom.TracesWithoutTree(tc))
	w.Testcase(ctx, ts, tc)
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	protocol, err := os.ReadFile(filepath.Join(run, "protocols", "receive-webhook-ok", "SB-01-01-ok-webhook.md"))
	require.NoError(t, err)
	assert.Contains(t, string(protocol), "svc-gateway · /v1/supabase/webhook/{integration_hash_ids}")
	assert.NotContains(t, string(protocol), "trace not saved")
	tree, err := os.ReadFile(filepath.Join(run, "protocols", "traces", traceOK+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(tree), "svc-gateway")
}
