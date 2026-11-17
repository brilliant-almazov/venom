package runartifacts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// hexTrace is a 32-hex trace id as the canon hook of the run folder reads it.
var hexTrace = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

// supabaseSuite is a testsuite of the case set: scenarios/supabase/<path>/SB-NN-NN-*.yml, two testcases of two steps.
func supabaseSuite(t *testing.T, root string) *venom.TestSuite {
	t.Helper()
	file := filepath.Join(root, "scenarios", "supabase", "identifier-link", "SB-01-01-link.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte("name: SB-01-01 link\n"), 0o644))
	step := func(name, typ string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"name": name, "type": typ})
		require.NoError(t, err)

		return raw
	}
	ts := &venom.TestSuite{Name: "SB-01-01 link", ShortName: "SB-01-01-link", Filepath: file}
	ts.TestCases = []venom.TestCase{
		{TestCaseInput: venom.TestCaseInput{Name: "SB-01-01-01 create link", RawTestSteps: []json.RawMessage{step("seed", "seed"), step("create", "http")}},
			SourceLine: 3, StepSourceLines: []int{5, 9}},
		{TestCaseInput: venom.TestCaseInput{Name: "SB-01-01-02 read link", RawTestSteps: []json.RawMessage{step("read", "http"), step("check", "check")}},
			SourceLine: 14, StepSourceLines: []int{16, 20}},
	}

	return ts
}

func readRun(t *testing.T, run, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(run, rel))
	require.NoError(t, err, rel)

	return string(body)
}

// A run killed while a testcase runs keeps the protocol of every finished testcase and the one under way marked
// interrupted, with INDEX.md, REPORT.md, PROBLEMS.md and the schemes as they stood: all written as each testcase began
// and ended, none at the end of the run.
func TestKilledRunLeavesInterrupted(t *testing.T) {
	root := t.TempDir()
	ts := supabaseSuite(t, root)
	traces := filepath.Join(root, "saved")
	require.NoError(t, os.MkdirAll(traces, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(traces, traceOK+".md"), []byte("svc ✅"), 0o644))
	ts.TestCases[0].Status = venom.StatusPass
	ts.TestCases[0].TestStepResults = []venom.TestStepResult{call(venom.StatusPass, 200, traceOK, traces)}
	ts.TestCases[0].TestStepResults[0].Number = 1

	run := filepath.Join(root, "run")
	w := New(run)
	ctx := context.Background()
	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})
	w.Start(ctx, ts, &ts.TestCases[0])
	w.Testcase(ctx, ts, &ts.TestCases[0])
	w.Start(ctx, ts, &ts.TestCases[1])

	assert.Contains(t, readRun(t, run, "protocols/sb-01-01-link/SB-01-01-01-ok-create-link.md"), "✅ passed")
	assert.NoFileExists(t, filepath.Join(run, "protocols/sb-01-01-link/SB-01-01-01-failed-create-link.md"),
		"the protocol written as the testcase began is renamed by its outcome, not left next to it")
	interrupted := readRun(t, run, "protocols/sb-01-01-link/SB-01-01-02-failed-read-link.md")
	assert.Contains(t, interrupted, "⏸ interrupted: the run ended before the testcase did")
	assert.Contains(t, interrupted, "scenarios/supabase/identifier-link/SB-01-01-link.yml:14](")
	assert.Contains(t, interrupted, "| 2 | check | check | ⏸ | not run: the testcase stopped before this step |")

	index := readRun(t, run, "protocols/INDEX.md")
	assert.Contains(t, index, "| SB-01-01 link | SB-01-01-01 create link | ✅ 200 |")
	assert.Contains(t, index, "| SB-01-01 link | SB-01-01-02 read link | ⏸ interrupted |")
	assert.Contains(t, readRun(t, run, ProblemsFile), "| SB-01-01-02 read link | identifier-link | ⏸ interrupted |")
	assert.Contains(t, readRun(t, run, ReportFile), "- end: under way")
	assert.Contains(t, readRun(t, run, "schemes/SYSTEM.md"), "SB-01-01-02 read link · ⏸ interrupted")
}

// A run that ends early — a signal, a setup error, a crash — names every testcase: the one under way interrupted with
// the reason, each one not started with a protocol «not run».
func TestAbortedRunNamesEveryTestcase(t *testing.T) {
	root := t.TempDir()
	ts := supabaseSuite(t, root)
	run := filepath.Join(root, "run")
	w := New(run)
	ctx := context.Background()
	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})
	w.Start(ctx, ts, &ts.TestCases[0])
	require.NoError(t, w.Abort(ctx, "interrupted by signal terminated"))

	assert.Contains(t, readRun(t, run, "protocols/sb-01-01-link/SB-01-01-01-failed-create-link.md"),
		"⏸ interrupted: interrupted by signal terminated")
	assert.Contains(t, readRun(t, run, "protocols/sb-01-01-link/SB-01-01-02-failed-read-link.md"),
		"⏸ not run: interrupted by signal terminated")
	assert.Contains(t, readRun(t, run, ProblemsFile), "- run ended early: interrupted by signal terminated")
	assert.Contains(t, readRun(t, run, ReportFile), "- run ended early: interrupted by signal terminated")

	w.Testcase(ctx, ts, &ts.TestCases[1])
	assert.Contains(t, readRun(t, run, "protocols/sb-01-01-link/SB-01-01-02-failed-read-link.md"), "⏸ not run")
}

// A setup error before any testcase ran still writes protocols/INDEX.md, REPORT.md and PROBLEMS.md with the error.
func TestSetupFailureWritesTheRunFolder(t *testing.T) {
	run := filepath.Join(t.TempDir(), "run")
	w := New(run)
	require.NoError(t, w.Abort(context.Background(), "setup failed: missing variables [x]"))

	assert.Contains(t, readRun(t, run, "protocols/INDEX.md"), indexHeader)
	assert.Contains(t, readRun(t, run, ProblemsFile), "setup failed: missing variables [x]")
	assert.Contains(t, readRun(t, run, ReportFile), "setup failed: missing variables [x]")
}

// The schemes are drawn from the run: the system, the path, the case; ok / not-ok on each transition with its reason,
// the checked cases with the request, the expected behaviour and the transitions; links both ways between the scheme,
// the case, the scenario line, the protocol and the trace id.
func TestSchemesDrawnFromTheRun(t *testing.T) {
	root := t.TempDir()
	ts := supabaseSuite(t, root)
	traces := filepath.Join(root, "saved")
	require.NoError(t, os.MkdirAll(traces, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(traces, traceOK+".md"), []byte("svc ✅"), 0o644))

	ts.TestCases[0].Status = venom.StatusPass
	ts.TestCases[0].TestStepResults = []venom.TestStepResult{call(venom.StatusPass, 200, traceOK, traces)}
	ts.TestCases[0].TestStepResults[0].Number = 1
	ts.TestCases[0].TestStepResults[0].ComputedInfo = []string{"expect: the link is created"}
	ts.TestCases[1].Status = venom.StatusFail
	ts.TestCases[1].TestStepResults = []venom.TestStepResult{call(venom.StatusFail, 409, traceLost, traces, "expected 200 got 409")}
	ts.TestCases[1].TestStepResults[0].Number = 1

	run := filepath.Join(root, "run")
	w := New(run)
	ctx := context.Background()
	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})

	for i := range ts.TestCases {
		w.Start(ctx, ts, &ts.TestCases[i])
		w.Testcase(ctx, ts, &ts.TestCases[i])
	}

	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	system := readRun(t, run, "schemes/SYSTEM.md")
	assert.Contains(t, system, "system · ❌ 1/2")
	assert.Contains(t, system, "├─ SB-01-01-01 create link · ✅ 200 `0af76519`")
	assert.Contains(t, system, "[identifier-link](identifier-link/PATH.md)")
	assert.Contains(t, system, "expect: the link is created")

	path := readRun(t, run, "schemes/identifier-link/PATH.md")
	assert.Contains(t, path, "- folder: `"+filepath.ToSlash(filepath.Dir(ts.Filepath))+"`")
	assert.Contains(t, path, "[SYSTEM.md](../SYSTEM.md)")
	assert.Contains(t, path, "[protocol](../../protocols/sb-01-01-link/SB-01-01-02-failed-read-link.md)")

	failedCase := readRun(t, run, "schemes/identifier-link/SB-01-01-02-read-link.md")
	assert.Contains(t, failedCase, "1 call · http ─► ❌ 409 `4bf92f35`")
	assert.Contains(t, failedCase, "reason: expected 200 got 409")
	assert.Contains(t, failedCase, "2 check · check ─► ⏸")
	assert.Contains(t, failedCase, "SB-01-01-link.yml:16](")
	assert.Contains(t, failedCase, "#L16)")
	assert.Contains(t, failedCase, "[protocol](../../protocols/sb-01-01-link/SB-01-01-02-failed-read-link.md)")
	assert.Regexp(t, hexTrace, failedCase)

	protocol := readRun(t, run, "protocols/sb-01-01-link/SB-01-01-01-ok-create-link.md")
	assert.Contains(t, protocol, "## Links")
	assert.Contains(t, protocol, "SB-01-01-link.yml:3](")
	assert.Contains(t, protocol, "- trace id: `"+traceOK+"` [span tree](../traces/"+traceOK+".md)")
	assert.Contains(t, protocol, "[SB-01-01-01 create link](../../schemes/identifier-link/SB-01-01-01-create-link.md)")
	assert.Contains(t, protocol, "[identifier-link](../../schemes/identifier-link/PATH.md)")

	problems := readRun(t, run, ProblemsFile)
	assert.Contains(t, problems, "| SB-01-01-02 read link | identifier-link | ❌ 409 | 1 call | expected 200 got 409 |")
	assert.NotContains(t, problems, "SB-01-01-01 create link")

	report := readRun(t, run, ReportFile)
	assert.Contains(t, report, "- verdict: ❌ 1/2")
	assert.Contains(t, report, "| `"+traceOK+"` |")
}

// No empty folder is left in the run folder: a folder stays only with a file in it.
func TestNoEmptyFolders(t *testing.T) {
	run := filepath.Join(t.TempDir(), "run")
	require.NoError(t, os.MkdirAll(filepath.Join(run, RawDir, "identifier-link", "traces"), 0o755))
	require.NoError(t, New(run).Finish(context.Background(), &venom.Tests{}))

	_ = filepath.WalkDir(run, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)

		if d.IsDir() {
			entries, readErr := os.ReadDir(path)
			require.NoError(t, readErr)
			assert.NotEmpty(t, entries, strings.TrimPrefix(path, run))
		}

		return nil
	})
	assert.NoDirExists(t, filepath.Join(run, RawDir, "identifier-link"))
}
