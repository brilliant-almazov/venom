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
