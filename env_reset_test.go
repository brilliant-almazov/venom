package venom

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// envSetFake registers the reset of svc-a with the bin of reset_bin (true when empty) and answers the code of its step.
type envSetFake struct{}

func (envSetFake) Run(ctx context.Context, step TestStep) (interface{}, error) {
	bin, _ := step.StringValue("reset_bin")
	if bin == "" {
		bin = "true"
	}
	if err := RegisterEnvReset(ctx, EnvReset{Service: "svc-a", Bin: bin}); err != nil {
		return nil, err
	}
	code, _ := step.StringValue("code")
	return map[string]interface{}{"code": code}, nil
}

func runEnvSuite(t *testing.T, suite string) *Venom {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "suite.yml")
	require.NoError(t, os.WriteFile(file, []byte(suite), 0o644))

	testLoggerOnce.Do(func() { InitTestLogger(t) })
	v := New()
	v.RegisterExecutorBuiltin(EnvSetType, envSetFake{})
	v.OutputDir = dir
	v.Parallel = 4
	v.PrintFunc = func(string, ...interface{}) (int, error) { return 0, nil }
	ctx := context.Background()
	require.NoError(t, v.Parse(ctx, []string{file}))
	require.NoError(t, v.Process(ctx, []string{file}))
	return v
}

const envSuiteFailedStep = `name: env
testcases:
- name: changes
  steps:
  - type: env_set
    code: "10"
    assertions:
    - code ShouldEqual 0
- name: after
  steps:
  - type: env_set
    code: "0"
    assertions:
    - code ShouldEqual 0
`

const envSuiteFailedReset = `name: env
testcases:
- name: changes
  steps:
  - type: env_set
    code: "0"
    reset_bin: "false"
- name: after
  steps:
  - type: env_set
    code: "0"
`

func TestEnvSetTestCaseResetsWhateverItsOutcome(t *testing.T) {
	v := runEnvSuite(t, envSuiteFailedStep)
	ts := v.Tests.TestSuites[0]
	changes := ts.TestCases[0]
	require.Equal(t, StatusFail, changes.Status)
	last := changes.TestStepResults[len(changes.TestStepResults)-1]
	require.Equal(t, "env reset svc-a", last.Name)
	require.Equal(t, StatusPass, last.Status)
	require.Equal(t, StatusPass, ts.TestCases[1].Status)
	require.Equal(t, 1, v.parallelFor(&ts))
}

func TestFailedResetStopsTheRun(t *testing.T) {
	v := runEnvSuite(t, envSuiteFailedReset)
	ts := v.Tests.TestSuites[0]
	changes := ts.TestCases[0]
	require.Equal(t, StatusFail, changes.Status)
	last := changes.TestStepResults[len(changes.TestStepResults)-1]
	require.Equal(t, "base state not restored: svc-a: rc=1", last.Errors[0].Value)
	require.Equal(t, StatusSkip, ts.TestCases[1].Status)
	require.Equal(t, envDirtyReason, ts.TestCases[1].Skipped[0].Value)
}

func TestRestoreEnvRunsEveryServiceOnceLastFirst(t *testing.T) {
	v := New()
	ctx, rs := v.beginEnvResets(context.Background())
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-a", Bin: "true"}))
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-b", Bin: "true"}))
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-a", Bin: "false"}))
	tc := &TestCase{}
	tc.RawTestSteps = append(tc.RawTestSteps, []byte(`{"type":"env_set"}`))
	v.restoreEnv(ctx, tc, rs)
	require.Len(t, tc.TestStepResults, 2)
	require.Equal(t, "env reset svc-b", tc.TestStepResults[0].Name)
	require.Equal(t, 2, tc.TestStepResults[0].Number)
	require.Equal(t, "env reset svc-a", tc.TestStepResults[1].Name)
	require.Equal(t, StatusPass, tc.TestStepResults[1].Status)
	require.False(t, v.env.isDirty())
	require.Empty(t, v.env.pending())
}

func TestRestoreEnvRunsOnACanceledTestCase(t *testing.T) {
	v := New()
	parent, cancel := context.WithCancel(context.Background())
	ctx, rs := v.beginEnvResets(parent)
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-a", Bin: "true"}))
	cancel()
	tc := &TestCase{}
	v.restoreEnv(ctx, tc, rs)
	require.Equal(t, StatusPass, tc.TestStepResults[0].Status)
}

func TestRestoreEnvOnSignalNamesWhatFailed(t *testing.T) {
	v := New()
	ctx, _ := v.beginEnvResets(context.Background())
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-a", Bin: "false"}))
	require.NoError(t, RegisterEnvReset(ctx, EnvReset{Service: "svc-b", Bin: "true"}))
	require.Equal(t, []string{"svc-a: rc=1"}, v.RestoreEnv())
}

func TestRegisterOutOfATestCaseIsRefused(t *testing.T) {
	require.ErrorIs(t, RegisterEnvReset(context.Background(), EnvReset{Service: "svc-a"}), ErrNoTestCase)
}

func TestSuitesWithEnvSetRunAlone(t *testing.T) {
	plain := TestSuite{TestCases: []TestCase{selectCase("a", `{"type":"http"}`)}}
	env := TestSuite{TestCases: []TestCase{selectCase("b", `{"type":"env_set","service":"svc-a"}`)}}
	require.False(t, suitesUseEnvSet([]TestSuite{plain}))
	require.True(t, suitesUseEnvSet([]TestSuite{plain, env}))
	v := New()
	v.ParallelSuites = 2
	v.Tests.TestSuites = []TestSuite{plain, env}
	require.Equal(t, 1, v.parallelSuitesFor())
}
