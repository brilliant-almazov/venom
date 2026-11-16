package venom

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTree writes files under root, each path relative to it.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
}

func newTestVenom(t *testing.T) *Venom {
	t.Helper()
	testLoggerOnce.Do(func() { InitTestLogger(t) })
	v := New()
	v.RegisterExecutorBuiltin("caller", callExecutor{})
	return v
}

const libSave = `executor: trace_save
input:
  trace_id: ""
steps:
- type: caller
  code: 200
`

// A suite in scenarios/billing/account finds the executors of scenarios/lib with no --lib-dir (run 16-4: «user executor
// "trace_save" not found - loaded executors are: []»).
func TestUserExecutorLibFoundUpFromSuite(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".git/HEAD":                               "ref: refs/heads/master\n",
		"scenarios/lib/trace_save.yml":            libSave,
		"scenarios/billing/account/suite.yml":     "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: trace_save\n    trace_id: x\n",
		"scenarios/billing/account/lib/other.yml": "executor: other\nsteps:\n- type: caller\n  code: 200\n",
	})
	v := newTestVenom(t)
	require.NoError(t, v.Parse(context.Background(), []string{filepath.Join(root, "scenarios/billing/account/suite.yml")}))
	assert.Contains(t, v.executorsUser, "trace_save")
	assert.Contains(t, v.executorsUser, "other")
}

// The nearest lib wins over a lib further up with an executor of the same name.
func TestUserExecutorNearestLibWins(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".git/HEAD":                    "ref: refs/heads/master\n",
		"scenarios/lib/trace_save.yml": libSave,
		"scenarios/billing/lib/a.yml":  strings.Replace(libSave, "code: 200", "code: 201", 1),
		"scenarios/billing/suite.yml":  "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: trace_save\n",
	})
	v := newTestVenom(t)
	require.NoError(t, v.Parse(context.Background(), []string{filepath.Join(root, "scenarios/billing/suite.yml")}))
	ux := v.executorsUser["trace_save"].(UserExecutor)
	real, err := filepath.EvalSymlinks(filepath.Join(root, "scenarios/billing/lib/a.yml"))
	require.NoError(t, err)
	assert.Equal(t, real, ux.Filename)
}

// A lib directory that is a symlink is walked: filepath.Walk does not descend a symlinked root.
func TestUserExecutorSymlinkedLib(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".git/HEAD":                   "ref: refs/heads/master\n",
		"shared/trace_save.yml":       libSave,
		"scenarios/billing/suite.yml": "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: trace_save\n",
	})
	require.NoError(t, os.Symlink(filepath.Join(root, "shared"), filepath.Join(root, "scenarios", "lib")))
	v := newTestVenom(t)
	require.NoError(t, v.Parse(context.Background(), []string{filepath.Join(root, "scenarios/billing/suite.yml")}))
	assert.Contains(t, v.executorsUser, "trace_save")
}

// The walk up stops at the repository root: a lib above it is not loaded.
func TestUserExecutorLibStopsAtRepoRoot(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"lib/outside.yml":                  "executor: outside\nsteps:\n- type: caller\n  code: 200\n",
		"repo/.git/HEAD":                   "ref: refs/heads/master\n",
		"repo/scenarios/billing/suite.yml": "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: caller\n    code: 200\n",
	})
	assert.Empty(t, suiteLibDirs(filepath.Join(root, "repo/scenarios/billing"), ""))
	v := newTestVenom(t)
	require.NoError(t, v.Parse(context.Background(), []string{filepath.Join(root, "repo/scenarios/billing/suite.yml")}))
	assert.NotContains(t, v.executorsUser, "outside")
}

func dryRun(t *testing.T, files map[string]string, suite string) error {
	t.Helper()
	root := t.TempDir()
	files[".git/HEAD"] = "ref: refs/heads/master\n"
	writeTree(t, root, files)
	v := newTestVenom(t)
	v.DryRun = true
	v.AddVariables(map[string]interface{}{"gw_url": "http://gw"})
	return v.Parse(context.Background(), []string{filepath.Join(root, suite)})
}

func TestDryRunGreen(t *testing.T) {
	err := dryRun(t, map[string]string{
		"scenarios/lib/trace_save.yml": libSave,
		"scenarios/billing/suite.yml": `name: suite
vars:
  account: 7
testcases:
- name: create
  steps:
  - type: caller
    code: 200
    url: '{{.gw_url}}/accounts/{{.account}}'
    vars:
      id:
        from: result.body
    info:
    - "fact: {{.result.statuscode}}"
  - type: trace_save
    trace_id: '{{.id}}'
  - type: caller
    code: 200
    url: '{{.unset | default "x"}}/{{.venom.testcase}}'
    range: [1, 2]
    body: '{{.value}}'
- name: read
  steps:
  - type: caller
    code: 200
    url: '{{.create.id}}'
`,
	}, "scenarios/billing/suite.yml")
	require.NoError(t, err)
}

func TestDryRunUnsetVarNamedByLine(t *testing.T) {
	err := dryRun(t, map[string]string{
		"scenarios/billing/suite.yml": `name: suite
testcases:
- name: create
  steps:
  - type: caller
    code: 200
  - type: caller
    code: 200
    url: '{{.gw_url}}/accounts/{{.account_id}}'
`,
	}, "scenarios/billing/suite.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "suite.yml:9: testcase \"create\" step 2: {{.account_id}} is set by no var")
}

// A var set by a later step is not set for an earlier one.
func TestDryRunVarOfLaterStep(t *testing.T) {
	err := dryRun(t, map[string]string{
		"suite.yml": `name: suite
testcases:
- name: a
  steps:
  - type: caller
    code: 200
    url: '{{.id}}'
  - type: caller
    code: 200
    vars:
      id:
        from: result.body
`,
	}, "suite.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "suite.yml:7: testcase \"a\" step 1: {{.id}}")
}

func TestDryRunUnknownExecutorNamedByLine(t *testing.T) {
	err := dryRun(t, map[string]string{
		"suite.yml": "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: caller\n    code: 200\n  - type: trace_save\n",
	}, "suite.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `suite.yml:7: testcase "a" step 2: user executor "trace_save" not found`)
}

func TestDryRunUnknownExecutorInLib(t *testing.T) {
	err := dryRun(t, map[string]string{
		"lib/save.yml": "executor: save\nsteps:\n- type: caller\n  code: 200\n- type: nowhere\n",
		"suite.yml":    "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: save\n",
	}, "suite.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `save.yml:5: executor "save" step 2: unknown executor "nowhere"`)
}

func TestDryRunUnparseableYAMLNamedByLine(t *testing.T) {
	err := dryRun(t, map[string]string{
		"suite.yml": "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: caller\n   code: 200\n",
	}, "suite.yml")
	require.Error(t, err)
	assert.Regexp(t, `suite\.yml:\d+: error while unmarshal file`, err.Error())
}

// dryRunForm dry-runs a file of testdata/dryrun-form put in dir, the check and wait steps run by the call stub.
func dryRunForm(t *testing.T, dir, name string) error {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "dryrun-form", name))
	require.NoError(t, err)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".git/HEAD":                    "ref: refs/heads/master\n",
		"scenarios/lib/trace_save.yml": libSave,
		dir + "/" + name:               string(body),
	})
	v := newTestVenom(t)
	v.RegisterExecutorBuiltin("check", callExecutor{})
	v.RegisterExecutorBuiltin("wait", callExecutor{})
	v.DryRun = true
	return v.Parse(context.Background(), []string{filepath.Join(root, dir, name)})
}

// A testcase of check and wait steps only ends with no call (run 16-6: «no trace: the testcase has no call»).
func TestDryRunTestCaseWithNoCall(t *testing.T) {
	err := dryRunForm(t, "scenarios/billing", "no-call.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no-call.yml:3: testcase "S-049 snapshot of the condition": testcase has no call step`)
}

// A failed Must… before the first call stops the testcase before the call (run 16-6, S-062…S-066).
func TestDryRunMustBeforeCall(t *testing.T) {
	err := dryRunForm(t, "scenarios/billing", "must-before-call.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `must-before-call.yml:9: testcase "S-062 currency pair": step 2 asserts MustNotEqual before the first call`)
}

// A call, trace_save and then Must… is green; a setup or teardown testcase is not checked, whatever its case.
func TestDryRunCallThenMustGreen(t *testing.T) {
	require.NoError(t, dryRunForm(t, "scenarios/billing", "green.yml"))
}

// The files of scenarios/preflight are not checked.
func TestDryRunPreflightNotChecked(t *testing.T) {
	require.NoError(t, dryRunForm(t, "scenarios/preflight", "no-call.yml"))
}

func TestMaskCredentials(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"
	in := "dsn postgresql://postgres:s3cr3t@pg-billing:5432/billing?sslmode=disable token Bearer " + jwt
	out := maskCredentials(in)
	assert.NotContains(t, out, "s3cr3t")
	assert.NotContains(t, out, jwt)
	assert.Contains(t, out, "postgresql://postgres:__hidden__@pg-billing:5432/billing")
	assert.Equal(t, "http://gw/v1/accounts", maskCredentials("http://gw/v1/accounts"))
}

// No secret declared: the DSN and the JWT of the vars do not reach the JSON output (run 16-4: 52 in one file).
func TestOutputMasksCredentialsWithNoSecret(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"
	dsn := "postgresql://postgres:s3cr3t@pg-billing:5432/billing"
	dir := t.TempDir()
	v := newTestVenom(t)
	v.OutputDir = dir
	v.OutputFormat = "json"
	v.Tests.TestSuites = []TestSuite{{
		Name: "suite", Filepath: "suite.yml",
		Vars: H{"billing_pg_dsn": dsn, "gw_token": jwt, "n": 7},
		TestCases: []TestCase{{
			TestCaseInput: TestCaseInput{Name: "a", Vars: H{"gw_admin_token": jwt}},
			IsEvaluated:   true,
			TestStepResults: []TestStepResult{{
				InputVars:    map[string]string{"billing_pg_dsn": dsn},
				ComputedVars: H{"result.systemout": "Bearer " + jwt},
				Interpolated: []byte("dsn: " + dsn),
				Errors:       []Failure{{Value: "connect " + dsn + " failed"}},
			}},
		}},
	}}
	require.NoError(t, v.OutputResult())
	data, err := os.ReadFile(filepath.Join(dir, "test_results_suite.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "s3cr3t")
	assert.NotContains(t, string(data), jwt)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, 7, v.Tests.TestSuites[0].Vars["n"])
}

// Every call of a testcase is one line of <protocol-dir>/io.jsonl, whatever its outcome, credentials hidden.
func TestProtocolIOLines(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: green
  steps:
  - type: caller
    code: 200
    url: /v1/accounts/1
    trace_dir: %s
  - type: gateway
    code: 403
    assertions:
    - result.statuscode ShouldEqual 200
- name: second
  steps:
  - type: checker
    out: no call
`, false)
	data, err := os.ReadFile(filepath.Join(dir, protocolIOFile))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	var first, second protocolIOLine
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	assert.Equal(t, "green", first.Testcase)
	assert.Equal(t, 1, first.Step)
	assert.Equal(t, "200", first.Code)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", first.TraceID)
	assert.Equal(t, 2, second.Step)
	assert.Equal(t, "403", second.Code)
	assert.Equal(t, "POST", second.Method)
	assert.Equal(t, StatusFail, second.Status)
	assert.NotEmpty(t, second.Errors)
	assert.NotContains(t, lines[1], "secret-token")
	assert.Contains(t, second.Response, "role_required")
}
