// Package wait waits a given time with no shell, and exposes that as the `wait` executor: the absence window before a
// read, in place of an exec sleep (owner 2026-11-15: scenarios are declarative only).
//
// `seconds` is a number or an interpolated string, fractions allowed; `reason` says what the wait is for and is
// printed in the protocol. The step makes no call, returns no trace_id and has no default assertion; it ends early when
// the testcase context is done.
package wait

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// Name of the executor.
const Name = "wait"

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor waits seconds.
type Executor struct {
	Seconds string `json:"seconds" yaml:"seconds" mapstructure:"seconds"`
	Reason  string `json:"reason" yaml:"reason" mapstructure:"reason"`
}

// Result of the executor.
type Result struct {
	Seconds float64 `json:"seconds" yaml:"seconds"`
	Reason  string  `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// Run waits the seconds of the step.
func (Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	s, err := strconv.ParseFloat(strings.TrimSpace(e.Seconds), 64)
	if err != nil || s < 0 {
		return nil, fmt.Errorf("wait: seconds %q is not a non-negative number", e.Seconds)
	}
	timer := time.NewTimer(time.Duration(s * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return Result{Seconds: s, Reason: e.Reason}, nil
}
