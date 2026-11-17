package runartifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// Every testcase carries its state against the previous run of the set that has it — previous outcome → current,
// improved, worse or same, «new» when no earlier run has it — in REPORT.md, INDEX.md, its protocol, its schemes and the
// live scheme; REPORT.md and INDEX.md carry the counts of every outcome against the previous run.
func TestDynamicsAgainstThePreviousRun(t *testing.T) {
	root := t.TempDir()
	runs := filepath.Join(root, "cases", "supabase", "runs", "2026-11")
	past := "# Run report\n\n- verdict: ❌ 0/2\n" +
		"- testcases: 2 testcases · ✅ 0 · ❌ 2 · ⏸ 0 · ok 0% · partial 0% · failed 100% · not-run 0%\n" +
		"\n" + headDynamics + "\n\n| testcase | previous run | previous | current | dynamics |\n| -- | -- | -- | -- | -- |\n" +
		"| SB-03-01-01 create | — | — | failed | new |\n"
	require.NoError(t, os.MkdirAll(filepath.Join(runs, "17-9"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runs, "17-9", ReportFile), []byte(past), 0o644))

	ts := suiteOfZoom(t, root)
	run := filepath.Join(runs, "17-13")
	w := New(run)
	ctx := context.Background()
	w.Plan(&venom.Tests{TestSuites: []venom.TestSuite{*ts}})
	w.Testcase(ctx, ts, &ts.TestCases[1])

	assert.Contains(t, readRun(t, run, "protocols/INDEX.md"), " | failed → pending · 2026-11/17-9 |\n")

	w.Testcase(ctx, ts, &ts.TestCases[0])
	require.NoError(t, w.Finish(ctx, &venom.Tests{}))

	const improved = "failed → ok · improved · 2026-11/17-9"
	const total = "- previous run: 2026-11/17-9 · ok 0 → 1 (+1) · partial 0 → 0 (0) · failed 2 → 1 (-1) · not-run 0 → 0 (0)\n"

	report := readRun(t, run, ReportFile)
	assert.Contains(t, report, total)
	assert.Contains(t, report, "| SB-03-01-01 create | 2026-11/17-9 | failed | ok | improved |\n")
	assert.Contains(t, report, "| SB-03-01-02 row | — | — | failed | new |\n")

	index := readRun(t, run, "protocols/INDEX.md")
	assert.Contains(t, index, total)
	assert.Contains(t, index, " | "+improved+" |\n")
	assert.Contains(t, index, " | new |\n")

	assert.Contains(t, readRun(t, run, "protocols/SB-03-01-partial-accounts/SB-03-01-01-ok-create.md"), "- previous: "+improved+"\n")
	assert.Contains(t, readRun(t, run, "schemes/03-suites/SB-03-01-accounts.md"), "- previous: "+improved+"\n")

	l := w.layout()
	assert.Equal(t, improved+" · new · —", fill{rows: l.rows, past: l.past}.text("⟦SB-03-01-01:previous⟧ · ⟦SB-03-01-02:previous⟧ · ⟦SB-09:previous⟧"))

	body, err := os.ReadFile(filepath.Join(run, ReportFile))
	require.NoError(t, err)
	again := readReport("2026-11/17-13", string(body))
	assert.Equal(t, venom.ProtocolOutcomeOK, pastOutcome(again.cases["SB-03-01-01 create"]))
	assert.Equal(t, venom.ProtocolOutcomeFailed, pastOutcome(again.cases["SB-03-01-02 row"]))
}

// improved, worse or same go by the rank of the outcomes: ok, partial, failed, not-run.
func TestDynamicsVerdict(t *testing.T) {
	assert.Equal(t, dynImproved, dynamicsVerdict(venom.ProtocolOutcomeFailed, venom.ProtocolOutcomePartial))
	assert.Equal(t, dynWorse, dynamicsVerdict(venom.ProtocolOutcomeOK, venom.ProtocolOutcomeNotRun))
	assert.Equal(t, dynSame, dynamicsVerdict(venom.ProtocolOutcomeFailed, venom.ProtocolOutcomeFailed))
	assert.Equal(t, venom.ProtocolOutcomeNotRun, pastOutcome(caseRun{mark: venom.MarkNotRun}))
	assert.Equal(t, venom.ProtocolOutcomePartial, pastOutcome(caseRun{mark: venom.MarkFailed, ok: 50}))
}
