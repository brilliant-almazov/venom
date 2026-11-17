package runartifacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// billingFailed is a Tempo trace whose gateway call failed in billing.v1.Accounts/Create with gRPC code 13.
func billingFailed(id string) string {
	return `{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-gateway"}}]},"scopeSpans":[{"spans":[
 {"traceId":"` + id + `","spanId":"a","name":"POST /v1/billing/accounts","startTimeUnixNano":"1","endTimeUnixNano":"9","status":{"code":2}}]}]},
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-billing"}}]},"scopeSpans":[{"spans":[
 {"traceId":"` + id + `","spanId":"b","parentSpanId":"a","name":"billing.v1.Accounts/Create","startTimeUnixNano":"2","endTimeUnixNano":"8",
  "attributes":[{"key":"rpc.grpc.status_code","value":{"intValue":"13"}}]}]}]}]}`
}

// The run folder has fixes/<service>/<NN>-<testcase>.md per fix: two testcases whose upstream billing failed with one
// reason share one fix that lists both, with what was expected and what came, the norm and the trace; a testcase with
// no trace lies in fixes/not-established with the reason; REPORT.md and PROBLEMS.md link every fix file.
func TestFixesFolderPerService(t *testing.T) {
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		if id != traceOK && id != traceLost {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(billingFailed(id)))
	}))
	defer tempo.Close()

	upstream := func(trace string) venom.TestStepResult {
		r := call(venom.StatusFail, 500, trace, "", "expected 200 got 500")
		r.Name = "create account"
		r.Interpolated = []byte("type: http_call\ntempo: " + tempo.URL + "\n")
		r.ComputedVars["result.body"] = `{"code":13,"message":"accounts: insert failed"}`
		r.ComputedInfo = []string{"expect: the account is created", "norm: cgw-billing: docs/api.md:12"}

		return r
	}
	monolith := venom.TestStepResult{Name: "monolith", Status: venom.StatusFail, ComputedVars: venom.H{"result.statuscode": 404},
		Interpolated: []byte("type: http\n"), Errors: []venom.Failure{{Value: "expected 200 got 404"}}}

	ts := &venom.TestSuite{Name: "SB-03-01 accounts", ShortName: "SB-03-01-accounts", Filepath: filepath.Join(t.TempDir(), "accounts", "suite.yml")}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-01 create"}, Status: venom.StatusFail, TestStepResults: []venom.TestStepResult{upstream(traceOK)}},
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-02 create again"}, Status: venom.StatusFail, TestStepResults: []venom.TestStepResult{upstream(traceLost)}},
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-03 monolith"}, Status: venom.StatusFail, TestStepResults: []venom.TestStepResult{monolith}},
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

	index := read("fixes/INDEX.md")
	assert.Contains(t, index, "| [svc-billing](svc-billing/INDEX.md) | 1 | 2 |")
	assert.Contains(t, index, "| [not established](not-established/INDEX.md) | 1 | 1 |")

	assert.Contains(t, read("fixes/svc-billing/INDEX.md"),
		"| [01-SB-03-01-01-create.md](01-SB-03-01-01-create.md) | SB-03-01-01 create; SB-03-01-02 create again | 1 create account | expected 200 got 500 |")

	billing := read("fixes/svc-billing/01-SB-03-01-01-create.md")
	for _, want := range []string{
		"- fix in: svc-billing · billing.v1.Accounts/Create\n",
		"- testcases: 2\n",
		"- service: svc-billing\n- span of the failure: billing.v1.Accounts/Create\n",
		"- trace id of SB-03-01-01 create: `" + traceOK + "` [span tree](../../protocols/traces/" + traceOK + ".md)\n",
		"- trace id of SB-03-01-02 create again: `" + traceLost + "` [span tree](../../protocols/traces/" + traceLost + ".md)\n",
		"### SB-03-01-01 create\n",
		"### SB-03-01-02 create again\n",
		"- expected: expect: the account is created\n",
		"- got: ❌ 500 · expected 200 got 500\n",
		`"message": "accounts: insert failed"`,
		"- SB-03-01-01 create: norm: cgw-billing: docs/api.md:12 — ",
		"- SB-03-01-02 create again: norm: cgw-billing: docs/api.md:12 — ",
		"- SB-03-01-01 create: [protocol](../../protocols/SB-03-01-failed-accounts/SB-03-01-01-failed-create.md)",
	} {
		assert.Contains(t, billing, want)
	}

	unknown := read("fixes/not-established/01-SB-03-01-03-monolith.md")
	assert.Contains(t, unknown, "- fix in: not established (the call returned no trace id)\n")
	assert.Contains(t, unknown, "- service: not established (the call returned no trace id)\n")
	assert.Contains(t, unknown, "- got: ❌ 404 · expected 200 got 404\n")
	assert.Contains(t, unknown, "- SB-03-01-03 monolith: "+noNorm+" — ")

	const billingFix = "[svc-billing · billing.v1.Accounts/Create](fixes/svc-billing/01-SB-03-01-01-create.md)"
	const unknownFix = "[not established (the call returned no trace id)](fixes/not-established/01-SB-03-01-03-monolith.md)"
	problems := read(ProblemsFile)
	assert.Regexp(t, `\| SB-03-01-02 create again \|.*\| `+regexp.QuoteMeta(billingFix)+` \|`, problems)
	assert.Regexp(t, `\| SB-03-01-03 monolith \|.*\| `+regexp.QuoteMeta(unknownFix)+` \|`, problems)
	assert.Contains(t, read(ReportFile), "| ❌ 0/3 | "+emptyBar+" | "+billingFix+"; "+unknownFix+" |")
}

// A red testcase with no fix file is refused: the report and the problems are not written without it.
func TestRedTestcaseWithoutFixFileRefused(t *testing.T) {
	r := row{suiteFile: "suite.yml", testcase: "S-001 create", state: stateDone, view: venom.ProtocolView{Mark: venom.MarkFailed}}
	l := layout{dir: t.TempDir(), rows: []row{r}, fileOf: map[string]string{}}

	_, err := l.fixLink(l.dir, r)
	require.EqualError(t, err, "testcase S-001 create did not pass and has no fix file")
	require.Error(t, l.writeProblems(New(l.dir)))
	require.Error(t, l.writeReport(New(l.dir)))
}
