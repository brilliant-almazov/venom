package runartifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// apiKey is a Supabase key the answer and the request carry, as run 17-5 printed it in its protocols and io.jsonl.
const apiKey = "sb_secret_0123456789abcdefghijklmnopqrstuv"

// codeExecutor answers with the code of its step, the trace id of its step when it has one and the Supabase key in its
// request and answer.
type codeExecutor struct{}

// Result is the answer of codeExecutor; Venom names the result vars after the type, result.statuscode.
type Result struct {
	StatusCode int               `json:"statuscode"`
	Body       string            `json:"body"`
	Headers    map[string]string `json:"headers"`
	TraceID    string            `json:"trace_id,omitempty"`
}

func (codeExecutor) Run(_ context.Context, step venom.TestStep) (interface{}, error) {
	code, _ := step.IntValue("code")
	trace, _ := step.StringValue("trace")

	return Result{StatusCode: code, Body: `{"apikey":"` + apiKey + `"}`, Headers: map[string]string{"Apikey": apiKey},
		TraceID: trace}, nil
}

// suiteOfEveryOutcome is a testsuite with a passed testcase traced in Tempo, a failed Should, a failed Must that stops
// before its later steps, a testcase skipped by its skip condition and a testcase with one assertion held of two.
const suiteOfEveryOutcome = `name: SB-02-01 outcomes
testcases:
- name: SB-02-01-01 green
  info:
  - "expect: the row is read"
  steps:
  - type: code
    code: 200
    trace: ` + traceOK + `
    trace_dir: %s
    assertions:
    - result.statuscode ShouldEqual 200
- name: SB-02-01-02 should
  steps:
  - type: code
    code: 409
    assertions:
    - result.statuscode ShouldEqual 200
- name: SB-02-01-03 must
  steps:
  - type: code
    code: 500
    assertions:
    - result.statuscode MustEqual 200
  - type: code
    code: 200
- name: SB-02-01-04 skipped
  skip:
  - 1 ShouldEqual 2
  steps:
  - type: code
    code: 200
- name: SB-02-01-05 half
  steps:
  - type: code
    code: 409
    assertions:
    - result.statuscode ShouldBeGreaterThan 0
    - result.statuscode ShouldEqual 200
`

// A whole run of Venom lays out a protocol for every outcome, each named ok, partial or failed after its row id — the
// skipped one too, nothing is skipped by its name — in the folder of the testsuite named by its outcome; the shares of
// the outcomes in % by the run, the testsuite and the path; every slice of the scheme with its share; the span tree of the traced call in the protocols; the schemes of the
// run with links to the scenario; the report and the problems; every credential as <secret>, no empty folder.
func TestRunOfEveryOutcome(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "scenarios", "supabase", "rows", "SB-02-01-outcomes.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	saved := filepath.Join(root, "saved")
	require.NoError(t, os.MkdirAll(saved, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(saved, traceOK+".md"), []byte("svc-billing ✅ "+apiKey), 0o644))
	require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf(suiteOfEveryOutcome, saved)), 0o644))

	run := filepath.Join(root, "run")
	w := New(run)
	v := venom.New()
	v.RegisterExecutorBuiltin("code", codeExecutor{})
	v.Artifacts = w
	v.OutputDir = w.RawPath()
	v.OutputFormat = "json"
	require.NoError(t, v.InitLogger())
	ctx := context.Background()
	require.NoError(t, v.Parse(ctx, []string{file}))
	w.Plan(&v.Tests)
	require.NoError(t, v.Process(ctx, []string{file}))
	require.NoError(t, v.OutputResult())
	require.NoError(t, w.Finish(ctx, &v.Tests))

	dir := filepath.Join(run, ProtocolsDir, "SB-02-01-partial-outcomes")

	for name, want := range map[string]string{
		"SB-02-01-01-ok-green":       "✅ passed",
		"SB-02-01-02-failed-should":  "❌ failed",
		"SB-02-01-03-failed-must":    "❌ failed",
		"SB-02-01-04-not-run-skipped": "⏭ skipped",
		"SB-02-01-05-partial-half":   "❌ failed",
	} {
		body, err := os.ReadFile(filepath.Join(dir, name+".md"))
		require.NoError(t, err, name)
		assert.Contains(t, string(body), want, name)
		assert.Contains(t, string(body), "SB-02-01-outcomes.yml:", name)
		assert.Contains(t, string(body), "../../schemes/rows/", name)
		assert.NotContains(t, string(body), apiKey, name)
	}

	// one protocol per testcase: the name carries ok, partial, failed or not-run after the row id, the one written as the testcase began is gone
	protocols, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, protocols, 5)

	for _, p := range protocols {
		assert.Regexp(t, `^SB-02-01-0[1-5]-(ok|partial|failed|not-run)-[a-z]+\.md$`, p.Name())
	}

	green := readRun(t, run, "protocols/SB-02-01-partial-outcomes/SB-02-01-01-ok-green.md")
	assert.Contains(t, green, "- trace id: `"+traceOK+"` [span tree](../traces/"+traceOK+".md)")
	assert.Contains(t, green, "svc-billing ✅")
	tree := readRun(t, run, "protocols/traces/"+traceOK+".md")
	assert.Contains(t, tree, "svc-billing ✅ <secret>")
	assert.Contains(t, readRun(t, run, "protocols/INDEX.md"), "(traces/"+traceOK+".md)")

	must := readRun(t, run, "schemes/rows/SB-02-01-03-must.md")
	assert.Contains(t, must, "2 · code ─► ⏸")
	assert.Contains(t, must, "#L21)")

	assert.Contains(t, readRun(t, run, "schemes/rows/SB-02-01-01-green.md"), "- expected: expect: the row is read")
	problems := readRun(t, run, ProblemsFile)

	for _, name := range []string{"SB-02-01-02 should", "SB-02-01-03 must", "SB-02-01-05 half"} {
		assert.Contains(t, problems, "| "+name+" |")
	}

	// a skipped testcase is no defect of the product
	assert.NotContains(t, problems, "SB-02-01-04 skipped")

	// the protocols folder of the testsuite carries its outcome: one ok, one partial, two failed, one not-run is partial
	assert.NoDirExists(t, filepath.Join(run, ProtocolsDir, "SB-02-01-failed-outcomes"))
	assert.NoDirExists(t, filepath.Join(run, ProtocolsDir, "SB-02-01-ok-outcomes"))

	// the shares of the outcomes in %: by the run, the testsuite and the path, next to the count of testcases
	const share = "5 testcases · ✅ 1 · ❌ 3 · ⏸ 1 · ok 20% · partial 20% · failed 40% · not-run 20%"
	report := readRun(t, run, ReportFile)
	assert.Contains(t, report, "- verdict: ❌ 1/5")
	assert.Contains(t, report, "- testcases: "+share+"\n")
	assert.Contains(t, report, "| SB-02-01 outcomes | [SB-02-01-partial-outcomes/](protocols/SB-02-01-partial-outcomes) | "+share+" | ❌ 1/5 |")
	assert.Contains(t, report, "| rows | `"+filepath.ToSlash(filepath.Dir(file))+"` | "+share+" |")

	// every slice of the scheme — system, path, case — with its verdict and share, linked from REPORT.md, INDEX.md and
	// each protocol
	assert.Contains(t, report, "| system | [SYSTEM.md](schemes/SYSTEM.md) | ❌ 1/5 | ok 20% · partial 20% · failed 40% · not-run 20% |")
	assert.Contains(t, report, "| path | [rows](schemes/rows/PATH.md) | ❌ 1/5 | ok 20% · partial 20% · failed 40% · not-run 20% |")
	assert.Contains(t, report, "| case | [SB-02-01-05 half](schemes/rows/SB-02-01-05-half.md) | ❌ 409 | ✅ 0% · ❌ 100% · ⏸ 0% |")
	assert.Contains(t, report, "| case | [SB-02-01-03 must](schemes/rows/SB-02-01-03-must.md) | ❌ 500 | ✅ 0% · ❌ 50% · ⏸ 50% |")

	index := readRun(t, run, "protocols/INDEX.md")
	assert.Contains(t, index, "- path [rows](../schemes/rows/PATH.md) · ❌ 1/5 · ok 20% · partial 20% · failed 40% · not-run 20%")
	assert.Contains(t, index, "- system [SYSTEM.md](../schemes/SYSTEM.md) · ❌ 1/5 · ok 20% · partial 20% · failed 40% · not-run 20%")
	assert.Contains(t, index, "[scheme](../schemes/rows/SB-02-01-05-half.md) | [rows](../schemes/rows/PATH.md) |")

	system := readRun(t, run, "schemes/SYSTEM.md")
	assert.Contains(t, system, "system · ❌ 1/5 · ok 20% · partial 20% · failed 40% · not-run 20%")
	assert.Contains(t, system, "└─ rows · ❌ 1/5 · ok 20% · partial 20% · failed 40% · not-run 20%")
	assert.Contains(t, system, "- testcases: "+share)
	path := readRun(t, run, "schemes/rows/PATH.md")
	assert.Contains(t, path, "rows · ❌ 1/5 · ok 20% · partial 20% · failed 40% · not-run 20%")
	assert.Contains(t, path, "- testcases: "+share)
	assert.Contains(t, path, "[protocol](../../protocols/SB-02-01-partial-outcomes/SB-02-01-05-partial-half.md)")
	half := readRun(t, run, "schemes/rows/SB-02-01-05-half.md")
	assert.Contains(t, half, "- outcome: partial\n- transitions share: ✅ 0% · ❌ 100% · ⏸ 0%")
	assert.Contains(t, half, "[protocol](../../protocols/SB-02-01-partial-outcomes/SB-02-01-05-partial-half.md)")

	for _, want := range []string{"- scheme of the case: [SB-02-01-01 green](../../schemes/rows/SB-02-01-01-green.md)",
		"- scheme of the path: [rows](../../schemes/rows/PATH.md)", "- scheme of the system: [SYSTEM.md](../../schemes/SYSTEM.md)"} {
		assert.Contains(t, green, want)
	}

	_ = filepath.WalkDir(run, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)

		if d.IsDir() {
			entries, readErr := os.ReadDir(path)
			require.NoError(t, readErr)
			assert.NotEmpty(t, entries, path)

			return nil
		}

		body, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.False(t, strings.Contains(string(body), apiKey), "a credential in %s", path)

		return nil
	})
}
