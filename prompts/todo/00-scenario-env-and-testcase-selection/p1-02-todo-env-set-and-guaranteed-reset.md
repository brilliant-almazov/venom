# p1-02 — step `env_set` and the reset of the base state after the testcase, whatever its outcome

## Iteration budget

| Turns | Context lift, tokens | Execution, tokens | Total, tokens | Lift share | Money, $ | Source of the number |
| -- | -- | -- | -- | -- | -- | -- |
| up to 50 turns | 9k tokens | 200k tokens | 209k tokens | 4.5 % | ≈ $29 | estimate: costs/COSTS.md, lead 6c68eb47 2026-11-17 — 327k paid for one executor with its core change and design; ready text here removes the design, ≈ 60 %; money at the COSTS.md ratio $0.14 per 1k paid |

## What to do

Repository `/Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom`, branch
`feat/env-set-and-testcase-selection`. p1-01 is done: `venom.go` already holds the field `TestCaseNames` and
`README-LOCAL.md` the section 32. Eleven files: five new, six changed.

Agreed argv of the stand utility (cgw-env-cli-5b, 2026-11-22): `bin/cli var set <service> KEY1=VAL1 KEY2=VAL2 --restart
--env <env>` returns once the restarted service is healthy (rc 0) or not (rc 10; 2 bad argument, 4 docker down,
1 declaration error); `bin/cli var reset <service> --restart --env <env>` brings the base values of the scenario back,
rc 0 with no restart when they already are. The env of the suites is the var `stand` (cgw-qa-scenarios-b3).

### 1. New file `env_reset.go`

```go
package venom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// EnvSetType is the step type that changes ENV values of a stand service and restarts it (executors/cli).
const EnvSetType = "env_set"

// envDirtyReason is why the testcases after a failed reset do not run.
const envDirtyReason = "===== base state not restored: the run stopped ====="

// ErrNoTestCase is returned when a reset is registered out of a running testcase.
var ErrNoTestCase = errors.New("env reset: no testcase running")

// EnvReset is the command that brings one stand service back to the base values of its scenario. The step that
// changes the values registers it before it runs; it runs once the testcase ends, whatever its outcome.
type EnvReset struct {
	Service string
	Bin     string
	Dir     string
	Argv    []string
}

type envResetsKey struct{}

// envResets are the resets of one testcase, one per service, in the order they were registered.
type envResets struct {
	mu     sync.Mutex
	resets []EnvReset
}

// RegisterEnvReset registers r on the testcase running in ctx; a second reset of the same service is dropped.
func RegisterEnvReset(ctx context.Context, r EnvReset) error {
	rs, ok := ctx.Value(envResetsKey{}).(*envResets)
	if !ok {
		return ErrNoTestCase
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	for _, have := range rs.resets {
		if have.Service == r.Service {
			return nil
		}
	}

	rs.resets = append(rs.resets, r)

	return nil
}

// reversed returns the resets, the last registered first.
func (rs *envResets) reversed() []EnvReset {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	out := make([]EnvReset, 0, len(rs.resets))
	for i := len(rs.resets) - 1; i >= 0; i-- {
		out = append(out, rs.resets[i])
	}

	return out
}

// run starts the reset with no shell and returns its stdout, its stderr and why it failed, empty when it exited 0.
func (r EnvReset) run(ctx context.Context) (string, string, string) {
	cmd := exec.CommandContext(ctx, r.Bin, r.Argv...)
	cmd.Dir = r.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	fault := ""
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			fault = "rc=" + strconv.Itoa(exit.ExitCode())
		} else {
			fault = err.Error()
		}
	}

	return strings.TrimRight(stdout.String(), "\n"), strings.TrimRight(stderr.String(), "\n"), fault
}

// usesEnvSet reports whether a testcase of ts holds an env_set step: it restarts a service the other testcases share,
// so its testsuite runs alone and in order.
func usesEnvSet(ts *TestSuite) bool {
	for i := range ts.TestCases {
		for _, raw := range ts.TestCases[i].RawTestSteps {
			var step struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &step) == nil && step.Type == EnvSetType {
				return true
			}
		}
	}

	return false
}

// suitesUseEnvSet reports whether a testsuite of the run holds an env_set step.
func suitesUseEnvSet(suites []TestSuite) bool {
	for i := range suites {
		if usesEnvSet(&suites[i]) {
			return true
		}
	}

	return false
}
```

### 2. New file `env_state.go`

```go
package venom

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// envState is the resets of the testcases running now, and whether a reset failed: the stand is then off its base
// state and the testcases after it do not run. The copies of Venom a parallel run makes share it.
type envState struct {
	mu      sync.Mutex
	running map[*envResets]struct{}
	dirty   bool
}

func (s *envState) begin() *envResets {
	rs := &envResets{}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running == nil {
		s.running = map[*envResets]struct{}{}
	}
	s.running[rs] = struct{}{}

	return rs
}

func (s *envState) end(rs *envResets) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, rs)
}

func (s *envState) markDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true
}

func (s *envState) isDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.dirty
}

// pending is the resets of every testcase running now, the last registered of each first.
func (s *envState) pending() []EnvReset {
	s.mu.Lock()
	defer s.mu.Unlock()

	var all []EnvReset
	for rs := range s.running {
		all = append(all, rs.reversed()...)
	}

	return all
}

// beginEnvResets gives ctx the resets of the testcase about to run.
func (v *Venom) beginEnvResets(ctx context.Context) (context.Context, *envResets) {
	rs := v.env.begin()

	return context.WithValue(ctx, envResetsKey{}, rs), rs
}

// restoreEnv runs the resets of tc, the last registered first, each as a step result after the steps of the scenario;
// a failed one fails tc and stops the run. It runs on a context that is never canceled: the reset goes even when the
// testcase was canceled.
func (v *Venom) restoreEnv(ctx context.Context, tc *TestCase, rs *envResets) {
	defer v.env.end(rs)

	ctx = context.WithoutCancel(ctx)

	for i, r := range rs.reversed() {
		start := time.Now()
		out, errOut, fault := r.run(ctx)
		res := TestStepResult{Name: "env reset " + r.Service, Number: len(tc.RawTestSteps) + i + 1, Status: StatusPass,
			Systemout: out, Systemerr: errOut, Start: start, End: time.Now()}
		res.Duration = res.End.Sub(res.Start).Seconds()

		if fault != "" {
			res.appendError(fmt.Errorf("base state not restored: %s: %s", r.Service, fault))
			v.env.markDirty()
		}

		tc.TestStepResults = append(tc.TestStepResults, res)
	}
}

// RestoreEnv runs at once the resets of every testcase still running, for a run ended by a signal before its
// testcases reach their end. It returns one line per service not brought back: "<service>: <why>".
func (v *Venom) RestoreEnv() []string {
	ctx := context.Background()

	var failed []string
	for _, r := range v.env.pending() {
		if _, _, fault := r.run(ctx); fault != "" {
			failed = append(failed, r.Service+": "+fault)
		}
	}

	return failed
}
```

### 3. New file `env_reset_test.go`

```go
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
    - result.code ShouldEqual 0
- name: after
  steps:
  - type: env_set
    code: "0"
    assertions:
    - result.code ShouldEqual 0
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
```

### 4. New file `executors/cli/env_set.go`

```go
package cli

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// EnvSetName is the executor that sets ENV values of one stand service and restarts it.
const EnvSetName = venom.EnvSetType

// standVar is the var naming the stand when the step gives no env; every runner passes --var stand=<env>.
const standVar = "stand"

// EnvSet sets ENV values of one stand service and restarts it through the stand utility, with no shell:
// bin var set <service> KEY=VAL… --restart --env <env>; the utility returns once the service is healthy, rc 0, or not,
// rc 10. Before the call it registers bin var reset <service> --restart --env <env> on the testcase: Venom runs it once
// the testcase ends, whatever its outcome, and on SIGINT or SIGTERM.
type EnvSet struct {
	Bin     string            `json:"bin" yaml:"bin" mapstructure:"bin"`
	Service string            `json:"service" yaml:"service" mapstructure:"service"`
	Values  map[string]string `json:"values" yaml:"values" mapstructure:"values"`
	Env     string            `json:"env" yaml:"env" mapstructure:"env"`
}

// NewEnvSet returns the env_set executor.
func NewEnvSet() venom.Executor {
	return &EnvSet{}
}

// ZeroValueResult returns an empty result.
func (EnvSet) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: the service took the values and is healthy.
func (EnvSet) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.code ShouldEqual 0"}}
}

// Run registers the reset of the service on the testcase, then sets its values and restarts it.
func (EnvSet) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e EnvSet
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	e.Env = e.envOf(ctx)
	set, reset, err := e.argv()
	if err != nil {
		return nil, err
	}
	bin := e.Bin
	if bin == "" {
		bin = DefaultBin
	}
	r := venom.EnvReset{Service: e.Service, Bin: bin, Dir: venom.StringVarFromCtx(ctx, workdirVar), Argv: reset}
	if err := venom.RegisterEnvReset(ctx, r); err != nil {
		return nil, err
	}
	return run(ctx, bin, set)
}

// envOf is the env of the step, the var stand when the step gives none.
func (e EnvSet) envOf(ctx context.Context) string {
	if e.Env != "" {
		return e.Env
	}
	return venom.StringVarFromCtx(ctx, standVar)
}

// argv is the argv of var set, the keys sorted, and of var reset.
func (e EnvSet) argv() ([]string, []string, error) {
	if e.Service == "" {
		return nil, nil, errors.New("env_set: service is empty")
	}
	if len(e.Values) == 0 {
		return nil, nil, errors.New("env_set: values is empty")
	}
	if e.Env == "" {
		return nil, nil, errors.New("env_set: env is empty and the var stand is not set")
	}
	set := []string{"var", "set", e.Service}
	for _, k := range slices.Sorted(maps.Keys(e.Values)) {
		set = append(set, k+"="+e.Values[k])
	}
	set = append(set, "--restart", "--env", e.Env)
	return set, []string{"var", "reset", e.Service, "--restart", "--env", e.Env}, nil
}
```

### 5. New file `executors/cli/env_set_test.go`

```go
package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

func TestEnvSetArgv(t *testing.T) {
	set, reset, err := EnvSet{Service: "svc-crawler", Values: map[string]string{"B": "2", "A": "1"}, Env: "stand-1"}.argv()
	require.NoError(t, err)
	assert.Equal(t, []string{"var", "set", "svc-crawler", "A=1", "B=2", "--restart", "--env", "stand-1"}, set)
	assert.Equal(t, []string{"var", "reset", "svc-crawler", "--restart", "--env", "stand-1"}, reset)
}

func TestEnvSetRefusesAnIncompleteStep(t *testing.T) {
	for _, e := range []EnvSet{
		{Values: map[string]string{"A": "1"}, Env: "stand-1"},
		{Service: "svc-crawler", Env: "stand-1"},
		{Service: "svc-crawler", Values: map[string]string{"A": "1"}},
	} {
		_, _, err := e.argv()
		require.Error(t, err)
	}
}

func TestEnvSetEnvFromTheStandVar(t *testing.T) {
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.stand"), "stand-7")
	assert.Equal(t, "stand-7", EnvSet{}.envOf(ctx))
	assert.Equal(t, "stand-1", EnvSet{Env: "stand-1"}.envOf(ctx))
}

func TestEnvSetOutOfATestCaseIsRefused(t *testing.T) {
	step := venom.TestStep{"bin": writeFake(t), "service": "svc-crawler", "values": map[string]interface{}{"A": 1}, "env": "stand-1"}
	_, err := NewEnvSet().Run(context.Background(), step)
	require.ErrorIs(t, err, venom.ErrNoTestCase)
}
```

### 6. `executors/cli/cli.go` — one runner for `cli` and `env_set`

Before:

```go
// Package cli runs the stand utility from declared fields with no script text, and exposes that as the `cli`, `seed`
// and `stand_status` executors (owner 2026-11-15: scenarios are declarative only).
```

After:

```go
// Package cli runs the stand utility from declared fields with no script text, and exposes that as the `cli`, `seed`,
// `stand_status` and `env_set` executors (owner 2026-11-15: scenarios are declarative only).
```

Before:

```go
// DefaultBin is the stand utility when the step gives no bin, relative to the testsuite workdir.
const DefaultBin = "bin/cli"
```

After:

```go
// DefaultBin is the stand utility when the step gives no bin, relative to the testsuite workdir.
const DefaultBin = "bin/cli"

// workdirVar is the var holding the testsuite workdir, where the utility starts.
const workdirVar = "venom.testsuite.workdir"
```

Before:

```go
	e.fixed = x.fixed
	argv, err := e.argv()
	if err != nil {
		return nil, err
	}
	bin := e.Bin
	if bin == "" {
		bin = DefaultBin
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = venom.StringVarFromCtx(ctx, "venom.testsuite.workdir")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := Result{Code: "0"}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, fmt.Errorf("cli: %s: %w", bin, err)
		}
		r.Code = strconv.Itoa(exit.ExitCode())
	}
	r.Systemout = venom.RemoveNotPrintableChar(strings.TrimRight(stdout.String(), "\n"))
	r.Systemerr = venom.RemoveNotPrintableChar(strings.TrimRight(stderr.String(), "\n"))
	var parsed interface{}
	if err := venom.JSONUnmarshal([]byte(r.Systemout), &parsed); err == nil {
		r.SystemoutJSON = parsed
	}
	if e.fixed == "" && strings.Fields(e.Action)[0] == healthAction {
		red := Red(r.Systemout + "\n" + r.Systemerr)
		r.Red, r.RedNew = strings.Join(red, "\n"), strings.Join(RedNew(red, e.Baseline), "\n")
	}
	if r.OutputFile, err = evidence.Write(ctx, e.OutputFile, r.Systemout+r.Systemerr); err != nil {
		return nil, err
	}
	return r, nil
}
```

After:

```go
	e.fixed = x.fixed
	argv, err := e.argv()
	if err != nil {
		return nil, err
	}
	bin := e.Bin
	if bin == "" {
		bin = DefaultBin
	}
	r, err := run(ctx, bin, argv)
	if err != nil {
		return nil, err
	}
	if e.fixed == "" && strings.Fields(e.Action)[0] == healthAction {
		red := Red(r.Systemout + "\n" + r.Systemerr)
		r.Red, r.RedNew = strings.Join(red, "\n"), strings.Join(RedNew(red, e.Baseline), "\n")
	}
	if r.OutputFile, err = evidence.Write(ctx, e.OutputFile, r.Systemout+r.Systemerr); err != nil {
		return nil, err
	}
	return r, nil
}

// run starts bin with argv in the testsuite workdir, with no shell: the result carries its stdout, its parsed JSON when
// it is JSON, its stderr and its exit code.
func run(ctx context.Context, bin string, argv []string) (Result, error) {
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = venom.StringVarFromCtx(ctx, workdirVar)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := Result{Code: "0"}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return Result{}, fmt.Errorf("cli: %s: %w", bin, err)
		}
		r.Code = strconv.Itoa(exit.ExitCode())
	}
	r.Systemout = venom.RemoveNotPrintableChar(strings.TrimRight(stdout.String(), "\n"))
	r.Systemerr = venom.RemoveNotPrintableChar(strings.TrimRight(stderr.String(), "\n"))
	var parsed interface{}
	if err := venom.JSONUnmarshal([]byte(r.Systemout), &parsed); err == nil {
		r.SystemoutJSON = parsed
	}
	return r, nil
}
```

### 7. `executors/registry.go` — the executor registered

Before:

```go
	cli.StandStatusName: cli.NewStandStatus,
```

After:

```go
	cli.StandStatusName: cli.NewStandStatus,
	cli.EnvSetName:      cli.NewEnvSet,
```

### 8. `venom.go` — the shared state

Before:

```go
		secrets:          map[string]interface{}{},
		OutputFormat:     "xml",
	}
	return v
}
```

After:

```go
		secrets:          map[string]interface{}{},
		OutputFormat:     "xml",
		env:              &envState{},
	}
	return v
}
```

Before:

```go
	// TestCaseNames, when set, keeps only these testcases of the testsuites read, with what they need (select.go).
	TestCaseNames []string
}
```

After:

```go
	// TestCaseNames, when set, keeps only these testcases of the testsuites read, with what they need (select.go).
	TestCaseNames []string
	// env is the resets of the testcases running now and whether one failed (env_state.go); a pointer, so the copies
	// a parallel run makes share it.
	env *envState
}
```

### 9. `process_testcase.go`, `process_testsuite.go`, `parallel.go`, `run_artifacts_view.go`

`process_testcase.go`, function `runTestCase`. Before:

```go
	Info(ctx, "Starting testcase")

	defer Info(ctx, "Ending testcase")
	// ##### RUN Test Steps Here
	v.runTestSteps(ctx, tc, nil)
}
```

After:

```go
	Info(ctx, "Starting testcase")

	defer Info(ctx, "Ending testcase")
	ctx, resets := v.beginEnvResets(ctx)
	defer v.restoreEnv(ctx, tc, resets)
	// ##### RUN Test Steps Here
	v.runTestSteps(ctx, tc, nil)
}
```

`process_testsuite.go`. Before:

```go
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		stop, export := v.runOneTestCase(ctx, ts, tc)
		if stop {
			markRemainingSkipped(ts)
```

After:

```go
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		if v.env.isDirty() {
			markRemainingSkipped(ts, envDirtyReason)
			for j := i; j < len(ts.TestCases); j++ {
				v.writeProtocol(ctx, ts, &ts.TestCases[j])
			}
			return
		}
		stop, export := v.runOneTestCase(ctx, ts, tc)
		if stop {
			markRemainingSkipped(ts, stopOnFailureReason)
```

`process_testsuite.go`. Before:

```go
// markRemainingSkipped implements --stop-on-failure: every testcase not run yet is skipped.
func markRemainingSkipped(ts *TestSuite) {
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		if tc.Status == "" {
			tc.Status = StatusSkip
			tc.IsEvaluated = true
			tc.Skipped = append(tc.Skipped, Skipped{Value: "===== stop-on-failure: enabled ====="})
```

After:

```go
// stopOnFailureReason is why the testcases after a failure do not run with --stop-on-failure.
const stopOnFailureReason = "===== stop-on-failure: enabled ====="

// markRemainingSkipped skips every testcase not run yet, for reason: --stop-on-failure or a base state not restored.
func markRemainingSkipped(ts *TestSuite, reason string) {
	for i := range ts.TestCases {
		tc := &ts.TestCases[i]
		if tc.Status == "" {
			tc.Status = StatusSkip
			tc.IsEvaluated = true
			tc.Skipped = append(tc.Skipped, Skipped{Value: reason})
```

`parallel.go`. Before:

```go
// The suite runs sequentially (1) when Parallel <= 1, when --stop-on-failure is set,
// when it has a single testcase, or when a testcase references a var exported by an
// earlier testcase of the same suite: a template "{{.<earlier testcase name>.<var>}}"
// found in its steps or skip conditions.
func (v *Venom) parallelFor(ts *TestSuite) int {
	if v.Parallel <= 1 || v.StopOnFailure || len(ts.TestCases) < 2 {
```

After:

```go
// The suite runs sequentially (1) when Parallel <= 1, when --stop-on-failure is set,
// when it has a single testcase, when a testcase holds an env_set step, or when a testcase
// references a var exported by an earlier testcase of the same suite: a template
// "{{.<earlier testcase name>.<var>}}" found in its steps or skip conditions.
func (v *Venom) parallelFor(ts *TestSuite) int {
	if v.Parallel <= 1 || v.StopOnFailure || len(ts.TestCases) < 2 || usesEnvSet(ts) {
```

`parallel.go`. Before:

```go
// with -vvv (CPU profiling is process-wide), with a single testsuite, or when a
// testsuite references a var named after a testcase of another testsuite
// ("{{.<testcase name>.<var>}}").
func (v *Venom) parallelSuitesFor() int {
	suites := v.Tests.TestSuites
	if v.ParallelSuites <= 1 || v.StopOnFailure || v.Verbose >= 3 || len(suites) < 2 {
		return 1
	}
	if suitesShareVars(suites) {
```

After:

```go
// with -vvv (CPU profiling is process-wide), with a single testsuite, when a testsuite
// holds an env_set step, or when a testsuite references a var named after a testcase of
// another testsuite ("{{.<testcase name>.<var>}}").
func (v *Venom) parallelSuitesFor() int {
	suites := v.Tests.TestSuites
	if v.ParallelSuites <= 1 || v.StopOnFailure || v.Verbose >= 3 || len(suites) < 2 {
		return 1
	}
	if suitesShareVars(suites) || suitesUseEnvSet(suites) {
```

`run_artifacts_view.go`, function `protocolStepViews`. Before:

```go
		number := protocolStepNumber(r, i)
		if number > reached {
			reached = number
		}
```

After:

```go
		number := protocolStepNumber(r, i)
		if number > reached && number <= len(tc.RawTestSteps) {
			reached = number
		}
```

### 10. `cmd/venom/run/run_folder.go` and `cmd/venom/run/cmd.go` — reset on SIGINT and SIGTERM

`run_folder.go`. Before:

```go
type runFolder struct {
	writer *runartifacts.Writer
}
```

After:

```go
type runFolder struct {
	writer *runartifacts.Writer
	v      *venom.Venom
}
```

`run_folder.go`. Before:

```go
// onSignal closes the run folder when the run is interrupted (SIGINT) or terminated (SIGTERM), then exits 128 and
// the signal number. A SIGKILL cannot be caught: the folder then holds what was written as each testcase began and ended.
func (f runFolder) onSignal() {
	if f.writer == nil {
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		s := <-ch
		f.abort("interrupted by signal " + s.String())
```

After:

```go
// onSignal ends a run interrupted (SIGINT) or terminated (SIGTERM): it brings every stand service a running testcase
// changed back to its base values, closes the run folder, then exits 128 and the signal number. A SIGKILL cannot be
// caught: the folder then holds what was written as each testcase began and ended.
func (f runFolder) onSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		s := <-ch
		for _, failed := range f.v.RestoreEnv() {
			fmt.Fprintf(os.Stderr, "base state not restored: %s\n", failed)
		}
		f.abort("interrupted by signal " + s.String())
```

`cmd.go`. Before:

```go
		folder := runFolder{writer: artifacts}
```

After:

```go
		folder := runFolder{writer: artifacts, v: v}
```

### 11. `README-LOCAL.md` — section 33 before `## Build`

Before:

````markdown
Point tests: `select_test.go`.

## Build
````

After:

````markdown
Point tests: `select_test.go`.

## 33. A testcase that changes the ENV of a service: `type: env_set`, reset whatever the outcome

```yaml
- name: BIL-21 crawler batch of five
  steps:
  - type: env_set
    service: svc-crawler
    values:
      CRAWLER_BATCH: "5"
  - type: grpc
    ...
```

| field | type | default | |
| -- | -- | -- | -- |
| `service` | string | — | required, the stand service |
| `values` | map | — | required, `KEY: value`, passed sorted by key |
| `env` | string | the var `stand` | the stand, passed as `--env <env>` |
| `bin` | string | `bin/cli` | the utility, relative to the testsuite workdir; started directly, never through a shell |

The step runs `bin/cli var set <service> KEY=value… --restart --env <env>`: the utility restarts the service and returns
once it is healthy, rc 0, or not, rc 10. Result as `type: cli`: `result.code` (default assertion
`result.code ShouldEqual 0`), `result.systemout`, `result.systemerr`.

Before the call the step registers `bin/cli var reset <service> --restart --env <env>` on its testcase, once per
service. When the testcase ends, passed, failed, its step in error or canceled, Venom runs every reset, the last
registered first, each as a step `env reset <service>` after the steps of the scenario. On SIGINT or SIGTERM Venom runs
the resets of the testcases still running before it exits, and prints `base state not restored: <service>: <why>` for
each that failed. A suite writes no reset step of its own.

| reset | effect |
| -- | -- |
| rc 0 | the step `env reset <service>` passes |
| rc ≠ 0 | it fails with `base state not restored: <service>: rc=<code>`, the testcase fails, every testcase after it in the run is skipped with `===== base state not restored: the run stopped =====` |

A testsuite holding an `env_set` step runs its testcases in order (`--parallel` ignored) and runs alone
(`--parallel-suites` ignored): the restarted service is shared. Point tests: `env_reset_test.go`,
`executors/cli/env_set_test.go`.

## Build
````

## Canon

| ID | Requirement | Skill or hook | How to check |
| -- | -- | -- | -- |
| K1 | one runner of the stand utility: `cli` and `env_set` both call `run` of `executors/cli/cli.go`; the workdir var is one const | `cgw-no-duplication` | `env_set.go` holds no `exec.Command` |
| K2 | no dead code, no unused argument | `cgw-no-unused-code` | every function of the new files has a caller |
| K3 | context in tests is `context.Background()` or derived from it; the reset runs on `context.WithoutCancel` | `cgw-test-context` | `TestRestoreEnvRunsOnACanceledTestCase` |
| K4 | no `time.Sleep` in any test | `cgw-test-context` | the new test files hold none |
| K5 | failures by sentinel: `ErrNoTestCase`, checked with `require.ErrorIs` | `cgw-failure-taxonomy` | `TestRegisterOutOfATestCaseIsRefused`, `TestEnvSetOutOfATestCaseIsRefused` |
| K6 | no new ENV key | `env-key-names-are-owners.cjs` | no `os.Getenv` in the diff |
| K7 | nothing runs locally: no tests, build, lint | `local-test-runs-forbidden.cjs` | no command in this iteration but `## Status` |
| K8 | no AI attribution anywhere | `no-ai-attribution-anywhere.cjs` | the hook passed every write |
| K9 | absolute paths in Bash | `bash-absolute-paths.cjs` | the hook passed every command |

## CHECK

| ID | Claim | Proof | State |
| -- | -- | -- | -- |
| C1 | a testcase whose step failed still resets the service it changed (positive) | `TestEnvSetTestCaseResetsWhateverItsOutcome` green in CI | ⬜ |
| C2 | a canceled testcase still resets (positive) | `TestRestoreEnvRunsOnACanceledTestCase` green in CI | ⬜ |
| C3 | a failed reset fails the testcase and stops the run (negative) | `TestFailedResetStopsTheRun` green in CI | ⬜ |
| C4 | a reset registered out of a testcase is refused, the set never runs (negative) | `TestRegisterOutOfATestCaseIsRefused`, `TestEnvSetOutOfATestCaseIsRefused` green in CI | ⬜ |
| C5 | SIGINT runs the pending resets and names the failed ones (positive and negative in one) | `TestRestoreEnvOnSignalNamesWhatFailed` green in CI | ⬜ |
| C6 | the argv is the agreed `var set … --restart --env` / `var reset … --restart --env` (positive); an incomplete step is refused (negative) | `TestEnvSetArgv`, `TestEnvSetRefusesAnIncompleteStep` green in CI | ⬜ |
| C7 | a testsuite with env_set runs in order and alone | `TestSuitesWithEnvSetRunAlone`, `TestEnvSetTestCaseResetsWhateverItsOutcome` (`parallelFor` = 1) | ⬜ |

## Done when

- `env_reset.go`, `env_state.go`, `env_reset_test.go`, `executors/cli/env_set.go`, `executors/cli/env_set_test.go` lie
  at their paths with the text of §1–§5;
- `executors/cli/cli.go`, `executors/registry.go`, `venom.go`, `process_testcase.go`, `process_testsuite.go`,
  `parallel.go`, `run_artifacts_view.go`, `cmd/venom/run/run_folder.go`, `cmd/venom/run/cmd.go`, `README-LOCAL.md` hold
  every "After" block of §6–§11 in place of its "Before" block.

## Status

```
node ~/.claude/hooks/lib/prompt-status.js /Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom/prompts/in_progress/00-scenario-env-and-testcase-selection/p1-02-in_progress-env-set-and-guaranteed-reset.md done
```
