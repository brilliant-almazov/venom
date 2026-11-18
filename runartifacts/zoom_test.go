package runartifacts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// emptyBar is the bar of a block where every testcase failed.
const emptyBar = "[░░░░░░░░░░░░░░░░░░░░] 0 %"

// noTestcaseBar is the bar of a block with no testcase: all not-run.
const noTestcaseBar = "[····················] 0 %"

func TestProgress(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    count
		want string
	}{
		{"largest remainder", count{all: 88, ok: 80, notOK: 8}, "[██████████████████░░] 91 %"},
		{"three glyphs", count{all: 10, ok: 5, partial: 3, notOK: 2}, "[██████████▒▒▒▒▒▒░░░░] 50 %"},
		{"a share gets a cell", count{all: 100, ok: 1, notOK: 99}, "[█░░░░░░░░░░░░░░░░░░░] 1 %"},
		{"partial from the largest", count{all: 100, ok: 98, partial: 1, notOK: 1}, "[██████████████████▒░] 98 %"},
		{"half up", count{all: 8, ok: 1, notOK: 7}, "[███░░░░░░░░░░░░░░░░░] 13 %"},
		{"no testcase", count{}, noTestcaseBar},
		{"every testcase ok", count{all: 3, ok: 3}, "[████████████████████] 100 %"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := progress(tc.c)
			assert.Equal(t, tc.want, got)
			cells := strings.TrimSuffix(strings.SplitN(got, "]", 2)[0], "[")
			assert.Equal(t, barCells, len([]rune(strings.TrimPrefix(cells, "["))))
		})
	}
}

func TestRunBefore(t *testing.T) {
	assert.True(t, runBefore("2026-11/17-9", "2026-11/17-13"))
	assert.True(t, runBefore("2026-10/30-2", "2026-11/1-1"))
	assert.False(t, runBefore("2026-11/17-13", "2026-11/17-13"))
	assert.False(t, runBefore("2026-11/17-16", "2026-11/17-14"))
}

// A REPORT.md before the bars, as the runs 17-13…17-16 carry it, and one with them read the same.
func TestReadReport(t *testing.T) {
	old := "# Run report\n\n- verdict: ❌ 35/88\n- testcases: 88 testcases · ✅ 35 · ❌ 53 · ⏸ 0 · ok 40% · partial 10% · failed 50%\n" +
		"\n## Paths\n\n| path | folder | testcases | verdict | scheme |\n| -- | -- | -- | -- | -- |\n" +
		"| rows | `scenarios/supabase/rows` | 4 testcases · ✅ 3 · ❌ 1 · ⏸ 0 · ok 75% · partial 0% · failed 25% | ❌ 3/4 | [rows](x) |\n" +
		"\n## Testsuites\n\n| testsuite | protocols | testcases | verdict | fix in |\n| -- | -- | -- | -- | -- |\n" +
		"| SB-01 a \\| b | [x/](x) | 2 testcases · ✅ 2 · ❌ 0 · ⏸ 0 · ok 100% · partial 0% · failed 0% | ✅ 2/2 | — |\n" +
		"\n## Schemes\n\n| slice | scheme | verdict | share |\n| -- | -- | -- | -- |\n| path | [rows](x) | ❌ 9/9 | ok 1% |\n"

	run := readReport("2026-11/17-13", old)
	assert.Equal(t, count{all: 88, passed: 35, failed: 53, ok: 35, partial: 9, notOK: 44}, run.total)
	assert.Equal(t, count{all: 4, passed: 3, failed: 1, ok: 3, notOK: 1}, run.paths["rows"])
	assert.Equal(t, count{all: 2, passed: 2, ok: 2}, run.suites["SB-01 a | b"])
	assert.Len(t, run.paths, 1)
}

// trace is a Tempo trace of the gateway calling the billing server, which writes a table and publishes, and its
// worker consuming the queue.
const trace = `{"batches":[` +
	`{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"cgw-gateway"}}]},"scopeSpans":[{"spans":[` +
	`{"spanId":"a","name":"POST /v1/accounts","kind":"SPAN_KIND_SERVER","startTimeUnixNano":"1","endTimeUnixNano":"9","attributes":[` +
	`{"key":"http.request.method","value":{"stringValue":"POST"}},{"key":"http.route","value":{"stringValue":"/v1/accounts"}}]},` +
	`{"spanId":"b","parentSpanId":"a","name":"billing.v1.Accounts/Create","kind":3,"startTimeUnixNano":"2","endTimeUnixNano":"8",` +
	`"attributes":[{"key":"rpc.method","value":{"stringValue":"billing.v1.Accounts/Create"}}]}]}]},` +
	`{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"cgw-billing-server"}}]},"scopeSpans":[{"spans":[` +
	`{"spanId":"c","parentSpanId":"b","name":"billing.v1.Accounts/Create","kind":2,"startTimeUnixNano":"3","endTimeUnixNano":"7",` +
	`"status":{"code":"STATUS_CODE_ERROR","message":"boom"},` +
	`"attributes":[{"key":"rpc.method","value":{"stringValue":"billing.v1.Accounts/Create"}}]},` +
	`{"spanId":"d","parentSpanId":"c","name":"INSERT","kind":3,"startTimeUnixNano":"4","endTimeUnixNano":"5","attributes":[` +
	`{"key":"db.query.text","value":{"stringValue":"INSERT INTO account (id) VALUES ($1)"}}]},` +
	`{"spanId":"e","parentSpanId":"c","name":"publish","kind":4,"startTimeUnixNano":"5","endTimeUnixNano":"6","attributes":[` +
	`{"key":"messaging.destination.name","value":{"stringValue":"billing.account.created"}}]}]}]},` +
	`{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"cgw-billing-worker"}}]},"scopeSpans":[{"spans":[` +
	`{"spanId":"f","parentSpanId":"e","name":"consume","kind":5,"startTimeUnixNano":"6","endTimeUnixNano":"7","attributes":[` +
	`{"key":"messaging.destination.name","value":{"stringValue":"billing.account.created"}}]}]}]}]}`

// suiteOfZoom is a testsuite of an http call with its trace and an sql check, the second testcase failed.
func suiteOfZoom(t *testing.T, root string) *venom.TestSuite {
	traces := filepath.Join(root, "saved")
	require.NoError(t, os.MkdirAll(traces, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(traces, traceOK+".md"), []byte("tree"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(traces, traceOK+".json"), []byte(trace), 0o644))

	http := call(venom.StatusPass, 201, traceOK, traces)
	http.ComputedVars["result.request.method"] = "POST"
	http.ComputedVars["result.request.url"] = "https://gw.stand/v1/accounts"
	http.Interpolated = []byte("type: http\ntrace_dir: " + traces + "\nassertions:\n  - result.statuscode ShouldEqual 201\n")
	sql := venom.TestStepResult{Name: "row", Status: venom.StatusPass, Number: 2,
		Interpolated: []byte("type: sql\ndriver: postgres\ndsn: postgres://u:secret@db:5432/billing\ncommands:\n" +
			"  - |\n    SELECT id\n    FROM account\n    WHERE id = 1\nassertions:\n  - result.queries.__len__ ShouldEqual 1\n")}
	failedSQL := sql
	failedSQL.Status = venom.StatusFail
	failedSQL.Errors = []venom.Failure{{Value: "expected 1 got 0"}}

	ts := &venom.TestSuite{Name: "SB-03-01 accounts", ShortName: "SB-03-01-accounts",
		Filepath: filepath.Join(root, "scenarios", "accounts", "SB-03-01-accounts.yml")}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-01 create"}, Status: venom.StatusPass,
			TestStepResults: []venom.TestStepResult{http, sql}},
		{TestCaseInput: venom.TestCaseInput{Name: "SB-03-01-02 row"}, Status: venom.StatusFail,
			TestStepResults: []venom.TestStepResult{failedSQL}},
	}

	return ts
}

// The zoom set is in schemes/ of the run folder after every finished testcase, with the history of the earlier runs of
// the set, the nodes of the steps, the processes of the traces and every step down to its SQL text.
func TestZoomSet(t *testing.T) {
	root := t.TempDir()
	runs := filepath.Join(root, "cases", "supabase", "runs", "2026-11")
	past := "# Run report\n\n- verdict: ❌ 0/2\n- testcases: 2 testcases · ✅ 0 · ❌ 2 · ⏸ 0 · ok 0% · partial 0% · failed 100%\n" +
		"\n## Testsuites\n\n| testsuite | protocols | testcases | verdict | fix in |\n| -- | -- | -- | -- | -- |\n" +
		"| SB-03-01 accounts | [x/](x) | 2 testcases · ✅ 0 · ❌ 2 · ⏸ 0 · ok 0% · partial 0% · failed 100% | ❌ 0/2 | — |\n"
	require.NoError(t, os.MkdirAll(filepath.Join(runs, "17-9"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runs, "17-9", ReportFile), []byte(past), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(runs, "17-20"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runs, "17-20", ReportFile), []byte(past), 0o644))

	ts := suiteOfZoom(t, root)
	run := filepath.Join(runs, "17-13")
	w := New(run)
	ctx := context.Background()
	read := func(rel string) string {
		body, err := os.ReadFile(filepath.Join(run, SchemesDir, rel))
		require.NoError(t, err, rel)

		return string(body)
	}

	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})
	w.Testcase(ctx, ts, &ts.TestCases[0])
	assert.Contains(t, read(ZoomRunFile), "run 2026-11/17-13  [██████████··········] 50 %  ⏸ 1/2")

	w.Testcase(ctx, ts, &ts.TestCases[1])
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	runText := read(ZoomRunFile)
	assert.Contains(t, runText, "2026-11/17-9              "+emptyBar+"  ❌ 0/2")
	assert.Contains(t, runText, "2026-11/17-13 (this run)  [██████████░░░░░░░░░░] 50 %  ❌ 1/2    +50.0      +1")
	assert.NotContains(t, runText, "17-20")

	system := read(ZoomSystemFile)
	assert.Contains(t, system, "├─► http gw.stand  [████████████████████] 100 %  ✅ 1/1")
	assert.Contains(t, system, "└─► sql postgres billing  [██████████░░░░░░░░░░] 50 %  ❌ 1/2  ◄ red")
	assert.Contains(t, system, "   └─ table account  ")
	assert.Contains(t, system, "| 2 | http gw.stand | sql postgres billing | 1 | ✅ 1/1 |")
	assert.Contains(t, system, "├─ in  grpc billing.v1.Accounts/Create  [████████████████████] 100 %  ✅ 1/1  ◄ red  · SB-03-01-01")
	assert.Contains(t, system, "├─ out table account written")
	assert.Contains(t, system, "└─ out queue billing.account.created")
	assert.Contains(t, system, "cgw-billing-worker  [████████████████████] 100 %  ✅ 1/1\n└─ in  queue billing.account.created")
	assert.Contains(t, system, "| cgw-gateway | in | http POST /v1/accounts | 1 | ✅ 1/1 | [████████████████████] 100 % | 0 | SB-03-01-01 | 100 % |")
	assert.NotContains(t, system, "secret")

	suite := read(filepath.Join(ZoomSuitesDir, "SB-03-01-accounts.md"))
	assert.Contains(t, suite, "2026-11/17-9              "+emptyBar+"  ❌ 0/2")
	assert.Contains(t, suite, "│     call: POST https://gw.stand/v1/accounts")
	assert.Contains(t, suite, "│     assert: result.statuscode ShouldEqual 201")
	assert.Contains(t, suite, "      sql:\n        SELECT id\n        FROM account\n        WHERE id = 1")
	assert.Contains(t, suite, "      reason: expected 1 got 0")
	assert.Contains(t, suite, "### Processes of SB-03-01-01")
	assert.Contains(t, suite, "SB-03-01-accounts.yml](../../../../../../../scenarios/accounts/SB-03-01-accounts.yml)")
	assert.Contains(t, suite, "| 1 | http | POST | `https://gw.stand/v1/accounts` | ✅ | 201 |")

	path := read(filepath.Join(ZoomPathsDir, "accounts.md"))
	assert.Contains(t, path, "testsuite SB-03-01 accounts  [██████████░░░░░░░░░░] 50 %  ❌ 1/2")
	assert.Contains(t, path, "└─ SB-03-01-02 row  [░░░░░░░░░░░░░░░░░░░░] 0 %  ❌")

	index := read(IndexFile)
	assert.Contains(t, index, "- [00-run.md](00-run.md) — run · [██████████░░░░░░░░░░] 50 %  ❌ 1/2")
	assert.Contains(t, index, "  - [02-paths/accounts.md](02-paths/accounts.md) — path")
	assert.Contains(t, index, "    - [03-suites/SB-03-01-accounts.md](03-suites/SB-03-01-accounts.md) — testsuite SB-03-01 accounts")

	report, err := os.ReadFile(filepath.Join(run, ReportFile))
	require.NoError(t, err)
	assert.Contains(t, string(report), "- verdict: ❌ 1/2 · [██████████░░░░░░░░░░] 50 %")
}

// The history of a point is read from the same zoom file of the earlier run.
func TestPointHistory(t *testing.T) {
	body := "## Processes\n\n| process | direction | point | testcases | verdict | bar | failed spans | cases | history |\n" +
		"| -- | -- | -- | -- | -- | -- | -- | -- | -- |\n" +
		"| cgw-gateway | in | http POST /v1/accounts | 4 | ❌ 1/4 | [█████░░░░░░░░░░░░░░░] 25 % | 0 | A B C D | 25 % |\n" +
		"\n## Nodes\n\n| cgw-x | in | y | 1 | ✅ 1/1 | [x] 100 % | 0 | A | 100 % |\n"
	points := readPoints(body)
	assert.Len(t, points, 1)

	key := "cgw-gateway\x00in\x00http POST /v1/accounts"
	assert.Equal(t, count{all: 4, passed: 1, failed: 3, ok: 1, notOK: 3}, points[key])
	assert.Equal(t, "25 % → 100 % (Δ +75.0)", pointHistory([]map[string]count{nil, points}, key, count{all: 2, ok: 2}))
}
