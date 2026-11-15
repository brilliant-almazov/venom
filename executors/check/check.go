// Package check asserts on a given value with no call and no shell, and exposes that as the `check` executor.
//
// The step takes `value`, an interpolated string, and returns it as result.value; when the string is JSON its parsed
// form is result.valuejson. It makes no call and carries no trace: the template puts it after the call and its
// trace_save, in place of an exec step that echoed a var for an assertion (owner 2026-11-15: scenarios are declarative
// only).
package check

import (
	"context"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// Name of the executor.
const Name = "check"

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor checks one value.
type Executor struct {
	Value string `json:"value" yaml:"value" mapstructure:"value"`
}

// Result of the executor.
type Result struct {
	Value     string      `json:"value" yaml:"value"`
	ValueJSON interface{} `json:"valuejson,omitempty" yaml:"valuejson,omitempty"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// Run returns the value and its parsed JSON.
func (Executor) Run(_ context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	r := Result{Value: e.Value}
	var parsed interface{}
	if err := venom.JSONUnmarshal([]byte(strings.TrimSpace(e.Value)), &parsed); err == nil {
		r.ValueJSON = parsed
	}
	return r, nil
}
