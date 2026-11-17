package runartifacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

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

	assert.Contains(t, read("protocols/partial-billing/S-001-ok-create.md"), "svc-billing ✅")
	assert.Contains(t, read("protocols/partial-billing/S-002-failed-conflict.md"), "- trace not saved: "+traceLost)
	assert.Equal(t, "svc-billing ✅", read("protocols/traces/"+traceOK+".md"))

	index := read("protocols/INDEX.md")
	assert.Contains(t, index, "| billing | S-001 create | ✅ 200 | [0af76519](traces/"+traceOK+".md) | [protocol](partial-billing/S-001-ok-create.md) |")
	assert.Contains(t, index, "| billing | S-002 conflict | ❌ 409 | trace not saved `"+traceLost+"` | [protocol](partial-billing/S-002-failed-conflict.md) |")

	filled := read("schemes/1-system/system.md")
	assert.Equal(t, "gateway ──► billing   ❌ 1/2\n"+
		"S-001: ✅ 200 `0af76519` hop ✅ 200 `0af76519` reason — [protocol](../../protocols/partial-billing/S-001-ok-create.md) [0af76519](../../protocols/traces/"+traceOK+".md)\n"+
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

// The scheme drawn at intake gets its progress bars after every testcase: a node over several case ids counts their
// testcases, a case of one testcase the steps of it; a case the run has not reached yet is failed.
func TestSchemeBarsAfterEveryTestcase(t *testing.T) {
	root := t.TempDir()
	set := filepath.Join(root, "cases", "billing")
	require.NoError(t, os.MkdirAll(filepath.Join(set, SchemesDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(set, SchemesDir, "SYSTEM.md"),
		[]byte("billing ⟦BL-01-01 BL-01-02:bar⟧\n├─ BL-01-01 ⟦BL-01-01:bar⟧\n└─ BL-01-02 ⟦BL-01-02:bar⟧\n"), 0o644))

	ts := &venom.TestSuite{Name: "BL-01 accounts", ShortName: "accounts", Filepath: filepath.Join(root, "scenarios", "suite.yml")}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "BL-01-01 create"}, Status: venom.StatusPass,
			TestStepResults: []venom.TestStepResult{call(venom.StatusPass, 201, "", "")}},
		{TestCaseInput: venom.TestCaseInput{Name: "BL-01-02 read"}, Status: venom.StatusFail,
			TestStepResults: []venom.TestStepResult{call(venom.StatusFail, 500, "", "")}},
	}

	run := filepath.Join(set, "runs", "2026-11", "18-1")
	w := New(run)
	ctx := context.Background()
	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})
	w.Testcase(ctx, ts, &ts.TestCases[0])

	body, err := os.ReadFile(filepath.Join(run, SchemesDir, "SYSTEM.md"))
	require.NoError(t, err)
	assert.Equal(t, "billing [██████████░░░░░░░░░░] 50 %  ⏸ 1/2\n"+
		"├─ BL-01-01 [████████████████████] 100 %  ✅ 201\n"+
		"└─ BL-01-02 [░░░░░░░░░░░░░░░░░░░░] 0 %  ⏸ pending\n", string(body))

	w.Testcase(ctx, ts, &ts.TestCases[1])
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	body, err = os.ReadFile(filepath.Join(run, SchemesDir, "SYSTEM.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "billing [██████████░░░░░░░░░░] 50 %  ❌ 1/2\n")
	assert.Contains(t, string(body), "└─ BL-01-02 [░░░░░░░░░░░░░░░░░░░░] 0 %  ❌ 500\n")
}

// The scheme of the suite drawn at intake lives in QA, <qa>/schemes/<suite>, and wins over a schemes folder of the
// scenario set: its filled copy, bars and all, is in <run>/schemes/ after every testcase.
func TestSchemeFromQA(t *testing.T) {
	root := t.TempDir()
	qa := filepath.Join(root, "cgw-qa")
	require.NoError(t, os.MkdirAll(filepath.Join(qa, SchemesDir, "supabase"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(qa, SchemesDir, "supabase", "1-system.md"),
		[]byte("gateway ⟦SB-01-01:bar⟧\n"), 0o644))
	scenarios := filepath.Join(root, "cgw-qa-scenarios", "scenarios", "supabase")
	require.NoError(t, os.MkdirAll(filepath.Join(scenarios, SchemesDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scenarios, SchemesDir, "old.md"), []byte("⟦SB-01-01⟧"), 0o644))

	ts := &venom.TestSuite{Name: "SB-01 webhook", ShortName: "webhook", Filepath: filepath.Join(scenarios, "a", "suite.yml")}
	ts.TestCases = []venom.TestCase{{TestCaseInput: venom.TestCaseInput{Name: "SB-01-01 receive"}, Status: venom.StatusPass,
		TestStepResults: []venom.TestStepResult{call(venom.StatusPass, 200, "", "")}}}

	run := filepath.Join(qa, "cases", "supabase", "runs", "2026-11", "18-1")
	w := New(run)
	ctx := context.Background()
	w.Testcase(ctx, ts, &ts.TestCases[0])

	body, err := os.ReadFile(filepath.Join(run, SchemesDir, "1-system.md"))
	require.NoError(t, err)
	assert.Equal(t, "gateway [████████████████████] 100 %  ✅ 200\n", string(body))
	assert.NoFileExists(t, filepath.Join(run, SchemesDir, "old.md"))
	assert.NoFileExists(t, filepath.Join(run, SchemesDir, SystemFile))
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))
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

	protocol, err := os.ReadFile(filepath.Join(run, "protocols", "ok-receive-webhook-ok", "SB-01-01-ok-webhook.md"))
	require.NoError(t, err)
	assert.Contains(t, string(protocol), "svc-gateway · /v1/supabase/webhook/{integration_hash_ids}")
	assert.NotContains(t, string(protocol), "trace not saved")
	tree, err := os.ReadFile(filepath.Join(run, "protocols", "traces", traceOK+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(tree), "svc-gateway")
}

// traceLate is the trace of the monolith webhook call of run 17-12 (SB-05-01): Tempo had its tree only after 10 s.
const traceLate = "ad817128211920d3d8990adef0cd4ebc"

// Every http_call step of a user executor with a trace id of its own has its span tree in the protocol, a tree Tempo
// ingests after 10 s too (run 17-12, SB-05-01 step 1: «trace ad817128… returned by the call has no saved span tree»).
func TestEveryStepTraceInProtocol(t *testing.T) {
	ready := time.Now().Add(11 * time.Second)
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		if id != traceOK && (id != traceLate || time.Now().Before(ready)) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-` + id[:4] + `"}}]},` +
			`"scopeSpans":[{"spans":[{"traceId":"` + id + `","spanId":"a","name":"/v1/webhook","startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`))
	}))
	defer tempo.Close()

	step := func(name, id string) venom.TestStepResult {
		r := call(venom.StatusPass, 200, id, "")
		r.Name = name
		r.Interpolated = []byte("type: http_call\ntempo: " + tempo.URL + "\ntraces: \"\"\n")

		return r
	}

	ts := &venom.TestSuite{Name: "SB-05-01", ShortName: "SB-05-01-monolith-webhook-receive-ok", Filepath: filepath.Join(t.TempDir(), "suite.yml")}
	ts.TestCases = []venom.TestCase{{TestCaseInput: venom.TestCaseInput{Name: "SB-05-01 webhook"}, Status: venom.StatusPass,
		TestStepResults: []venom.TestStepResult{step("monolith webhook", traceLate), step("gateway read", traceOK)}}}

	run := filepath.Join(t.TempDir(), "run")
	w := New(run)
	ctx := context.Background()
	tc := &ts.TestCases[0]
	w.BeforeVerdict(ctx, ts, tc)
	assert.Empty(t, venom.TracesWithoutTree(tc))
	w.Testcase(ctx, ts, tc)
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	protocol, err := os.ReadFile(filepath.Join(run, "protocols", "SB-05-01-ok-monolith-webhook-receive-ok", "SB-05-01-ok-webhook.md"))
	require.NoError(t, err)
	assert.Contains(t, string(protocol), "svc-ad81 · /v1/webhook")
	assert.Contains(t, string(protocol), "svc-0af7 · /v1/webhook")
	assert.NotContains(t, string(protocol), "trace not saved")

	for _, id := range []string{traceLate, traceOK} {
		tree, err := os.ReadFile(filepath.Join(run, "protocols", "traces", id+".md"))
		require.NoError(t, err, id)
		assert.Contains(t, string(tree), "svc-"+id[:4])
	}
}

// The report names the service to fix: a call whose trace failed deep in billing names svc-billing with its span, a
// call with no trace names none with the reason — in the protocol, PROBLEMS.md and the testsuite row of REPORT.md
// explicitly.
func TestReportNamesServiceToFix(t *testing.T) {
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces/"+traceOK {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-gateway"}}]},"scopeSpans":[{"spans":[
 {"traceId":"` + traceOK + `","spanId":"a","name":"POST /v1/billing/accounts","startTimeUnixNano":"1","endTimeUnixNano":"9","status":{"code":2}}]}]},
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-billing"}}]},"scopeSpans":[{"spans":[
 {"traceId":"` + traceOK + `","spanId":"b","parentSpanId":"a","name":"billing.v1.Accounts/Create","startTimeUnixNano":"2","endTimeUnixNano":"8",
  "attributes":[{"key":"rpc.grpc.status_code","value":{"intValue":"13"}}]}]}]}]}`))
	}))
	defer tempo.Close()

	upstream := call(venom.StatusFail, 500, traceOK, "", "expected 200 got 500")
	upstream.Name = "http_call"
	upstream.Interpolated = []byte("type: http_call\ntempo: " + tempo.URL + "\n")
	monolith := venom.TestStepResult{Name: "monolith", Status: venom.StatusFail, ComputedVars: venom.H{"result.statuscode": 500},
		Interpolated: []byte("type: http\n"), Errors: []venom.Failure{{Value: "expected 200 got 500"}}}

	ts := &venom.TestSuite{Name: "SB-03-01 accounts", ShortName: "SB-03-01-accounts", Filepath: filepath.Join(t.TempDir(), "accounts", "suite.yml")}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-01 create"}, Status: venom.StatusFail, TestStepResults: []venom.TestStepResult{upstream}},
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-02 monolith"}, Status: venom.StatusFail, TestStepResults: []venom.TestStepResult{monolith}},
	}

	run := filepath.Join(t.TempDir(), "run")
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

	const billing = "svc-billing · billing.v1.Accounts/Create"
	const unknown = "not established (the call returned no trace id)"
	assert.Contains(t, read("protocols/SB-03-01-failed-accounts/SB-03-01-01-failed-create.md"), "\n\nfix in: "+billing+"\n\n## Links")
	assert.Contains(t, read("protocols/SB-03-01-failed-accounts/SB-03-01-02-failed-monolith.md"), "\n\nfix in: "+unknown+"\n\n## Links")

	const billingFix = "[" + billing + "](fixes/svc-billing/01-SB-03-01-01-create.md)"
	const unknownFix = "[" + unknown + "](fixes/not-established/01-SB-03-01-02-monolith.md)"
	problems := read(ProblemsFile)
	assert.Contains(t, problems, "| trace | fix in |")
	assert.Regexp(t, `\| SB-03-01-01 create \|.*\| `+regexp.QuoteMeta(billingFix)+` \|`, problems)
	assert.Regexp(t, `\| SB-03-01-02 monolith \|.*\| `+regexp.QuoteMeta(unknownFix)+` \|`, problems)

	assert.Contains(t, read(ReportFile), "| ❌ 0/2 | "+emptyBar+" | "+billingFix+"; "+unknownFix+" |")
}
