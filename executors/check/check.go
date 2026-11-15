// Package check asserts on a given value with no call and no shell, and exposes that as the `check` executor.
//
// The step takes `value`, an interpolated string, and returns it as result.value; when the string is JSON its parsed
// form is result.valuejson. It makes no call and carries no trace: the template puts it after the call and its
// trace_save, in place of an exec step that echoed a var for an assertion (owner 2026-11-15: scenarios are declarative
// only).
//
// `find_all` (FindAll) gives every match of a regexp over the value, `equal_json` (EqualJSON) compares the value with
// another JSON.
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
	// FindAll is a Go regexp: every match over value, or its first capture group when it has one.
	FindAll string `json:"find_all" yaml:"find_all" mapstructure:"find_all"`
	// Replace is [<regexp>, <replacement>], applied to each match of FindAll ($1 expands a group).
	Replace []string `json:"replace" yaml:"replace" mapstructure:"replace"`
	// Duplicates gives the matches that occur more than once.
	Duplicates bool `json:"duplicates" yaml:"duplicates" mapstructure:"duplicates"`
	// Join, when set, joins the matches by it into result.joined.
	Join string `json:"join" yaml:"join" mapstructure:"join"`
	// EqualJSON is a JSON compared with value; SortLists compares every list as a set of its items in any order.
	EqualJSON string `json:"equal_json" yaml:"equal_json" mapstructure:"equal_json"`
	SortLists bool   `json:"sort_lists" yaml:"sort_lists" mapstructure:"sort_lists"`
}

// Result of the executor.
type Result struct {
	Value     string      `json:"value" yaml:"value"`
	ValueJSON interface{} `json:"valuejson,omitempty" yaml:"valuejson,omitempty"`
	// Count, Matches (a JSON array), MatchesJSON, Duplicates (a JSON array) and Joined are set by find_all.
	Count       int           `json:"count" yaml:"count"`
	Matches     string        `json:"matches,omitempty" yaml:"matches,omitempty"`
	MatchesJSON []interface{} `json:"matchesjson,omitempty" yaml:"matchesjson,omitempty"`
	Duplicates  string        `json:"duplicates,omitempty" yaml:"duplicates,omitempty"`
	Joined      string        `json:"joined,omitempty" yaml:"joined,omitempty"`
	// JSONEqual and JSONDiff are set by equal_json.
	JSONEqual bool   `json:"json_equal" yaml:"json_equal"`
	JSONDiff  string `json:"json_diff,omitempty" yaml:"json_diff,omitempty"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// Run returns the value and its parsed JSON, the matches of find_all and the comparison of equal_json.
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
	if e.FindAll != "" {
		if err := e.findAll(&r); err != nil {
			return nil, err
		}
	}
	if e.EqualJSON != "" {
		equal, diff, err := EqualJSON(e.Value, e.EqualJSON, e.SortLists)
		if err != nil {
			return nil, err
		}
		r.JSONEqual, r.JSONDiff = equal, diff
	}
	return r, nil
}
