package venom

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/rockbears/yaml"
)

// Scenarios are declarative only, with no exception (owner 2026-11-15): a testsuite or a user executor with a
// `type: exec` step or a `script:` body is rejected at load, each such step named by file, testcase and step.

// declarativeLine finds an exec type or a script body in a file that does not parse as YAML before interpolation.
var declarativeLine = regexp.MustCompile(`^\s*(?:-\s+)?(?:type\s*:\s*["']?exec["']?\s*(?:#.*)?$|script\s*:)`)

// declarativeStep names what makes a step code: its exec type or its script body; empty for a declarative step.
func declarativeStep(raw json.RawMessage) string {
	var step map[string]interface{}
	if json.Unmarshal(raw, &step) != nil {
		return ""
	}
	var found []string
	if t, _ := step["type"].(string); strings.TrimSpace(t) == "exec" {
		found = append(found, "type: exec")
	}
	if _, ok := step["script"]; ok {
		found = append(found, "script:")
	}
	return strings.Join(found, ", ")
}

// lintTestSuite rejects the exec and script steps of the testcases of a testsuite file.
func lintTestSuite(file string, ts TestSuiteInput) error {
	var bad []string
	for _, tc := range ts.TestCases {
		for i, raw := range tc.RawTestSteps {
			if what := declarativeStep(raw); what != "" {
				bad = append(bad, fmt.Sprintf("%s: testcase %q step %d: %s", file, tc.Name, i+1, what))
			}
		}
	}
	return declarativeError(bad)
}

// lintUserExecutor rejects the exec and script steps of a user executor file; a file that does not parse before
// interpolation is read line by line.
func lintUserExecutor(file, name string, content []byte) error {
	var ux struct {
		Steps []json.RawMessage `json:"steps" yaml:"steps"`
	}
	var bad []string
	if yaml.Unmarshal(content, &ux) == nil {
		for i, raw := range ux.Steps {
			if what := declarativeStep(raw); what != "" {
				bad = append(bad, fmt.Sprintf("%s: executor %q step %d: %s", file, name, i+1, what))
			}
		}
		return declarativeError(bad)
	}
	for n, l := range strings.Split(string(content), "\n") {
		if declarativeLine.MatchString(l) {
			bad = append(bad, fmt.Sprintf("%s:%d: executor %q: %s", file, n+1, name, strings.TrimSpace(l)))
		}
	}
	return declarativeError(bad)
}

func declarativeError(bad []string) error {
	if len(bad) == 0 {
		return nil
	}
	return errors.Errorf("declarative only, no exec step and no script body:\n%s", strings.Join(bad, "\n"))
}
