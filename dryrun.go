package venom

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	yamlv3 "gopkg.in/yaml.v3"
)

// --dry-run loads every testsuite as a run does — YAML parsed, user executors loaded, every step type resolved to an
// executor — and checks every {{.x}} left in a step after the testsuite vars: it must be set by the testcase vars, by
// the vars or extracts of an earlier step, by an earlier testcase ({{.<testcase>.<var>}}) or by venom itself; every
// testcase but setup and teardown holds a call step with no Must… before it (dryRunTestCaseForm). No step
// runs and nothing touches the network. The first failure is named <file>:<line>.

// dryRunExpression is a template action left in a step; dryRunVarRef a variable it reads, .name or .name.sub.
var (
	dryRunExpression = regexp.MustCompile(`{{(.*?)}}`)
	dryRunVarRef     = regexp.MustCompile(`(?:^|[\s(|])\.([A-Za-z0-9_µ-][A-Za-z0-9_.µ-]*)`)
	dryRunYAMLLine   = regexp.MustCompile(`line (\d+)`)
)

// dryRunNoCheck are the helpers that give an unset variable a value, so the expression holds whatever is set.
var dryRunNoCheck = []string{"default", "ternary", "coalesce"}

// dryRunRangeVars are set by a step with range: for each of its items.
var dryRunRangeVars = []string{"index", "key", "value"}

// dryRunTestCase checks the vars of the steps of one testcase; called before the testcase vars are replaced by the
// testsuite vars (parseTestCases). earlier are the names of the testcases before it in the file.
func dryRunTestCase(ts *TestSuite, tc *TestCase, earlier []string) error {
	known := map[string]bool{}
	for k := range ts.Vars {
		known[k] = true
	}
	for k := range tc.Vars {
		known[k] = true
	}
	for _, name := range earlier {
		known[name] = true
	}
	for i, raw := range tc.RawTestSteps {
		var step map[string]interface{}
		if err := json.Unmarshal(raw, &step); err != nil {
			return dryRunErr(ts, tc, i, "", fmt.Sprintf("step does not parse: %v", err))
		}
		// an expression the testsuite vars could not fill prints <no value>, JSON-escaped in the raw step
		if strings.Contains(string(raw), "<no value>") || strings.Contains(string(raw), `\u003cno value\u003e`) {
			return dryRunErr(ts, tc, i, "", "an expression of the step reads a var that is not set: <no value>")
		}
		stepKnown := map[string]bool{}
		if _, ok := step["range"]; ok {
			for _, k := range dryRunRangeVars {
				stepKnown[k] = true
			}
		}
		for key, val := range step {
			// the vars, info and assertions of a step are read after it ran, with its result
			after := key == "vars" || key == "info" || key == "assertions"
			for _, ref := range dryRunRefs(val) {
				if dryRunKnown(ref, known) || dryRunKnown(ref, stepKnown) || (after && strings.HasPrefix(ref, "result.")) {
					continue
				}
				return dryRunErr(ts, tc, i, ref, fmt.Sprintf("{{.%s}} is set by no var of the testsuite or testcase, no earlier step and no earlier testcase", ref))
			}
		}
		for _, block := range []string{"vars", "extracts"} {
			if m, ok := step[block].(map[string]interface{}); ok {
				for k := range m {
					known[k] = true
					known[tc.Name+"."+k] = true
				}
			}
		}
	}
	return nil
}

// dryRunRefs are the variables read by the template actions of a value, an action with a default left out.
func dryRunRefs(val interface{}) []string {
	text, ok := val.(string)
	if !ok {
		raw, err := json.Marshal(val)
		if err != nil {
			return nil
		}
		text = string(raw)
	}
	var refs []string
	for _, m := range dryRunExpression.FindAllStringSubmatch(text, -1) {
		expr := strings.ReplaceAll(m[1], `\"`, `"`)
		if dryRunHasDefault(expr) {
			continue
		}
		for _, r := range dryRunVarRef.FindAllStringSubmatch(expr, -1) {
			refs = append(refs, strings.TrimSuffix(r[1], "."))
		}
	}
	return refs
}

// dryRunNoCall are the step types that call nothing: a step of any other type (http, grpc, sql, amqp, a user executor,
// …) is a call. dryRunMust is a Must… assertion: a failed one stops the testcase.
var (
	dryRunNoCall = map[string]bool{"check": true, "wait": true, "trace_save": true}
	dryRunMust   = regexp.MustCompile(`(?:^|\s)(Must[A-Z]\w*)`)
)

// dryRunTestCaseForm checks that a testcase cannot end with no call (run 16-6: 16 protocols «no trace: the testcase
// has no call»): it holds a call step, and no step before the first call asserts Must…, a failed one would stop the
// testcase before the call. A setup… or teardown… testcase and the files of scenarios/preflight are not checked.
func dryRunTestCaseForm(ts *TestSuite, tc *TestCase) error {
	if isStandCase(ts, tc) {
		return nil
	}
	for i, raw := range tc.RawTestSteps {
		var step map[string]interface{}
		if err := json.Unmarshal(raw, &step); err != nil {
			return dryRunErr(ts, tc, i, "", fmt.Sprintf("step does not parse: %v", err))
		}
		if typ, _ := step["type"].(string); !dryRunNoCall[typ] {
			return nil
		}
		if must := dryRunMustOf(step["assertions"]); must != "" {
			return errors.Errorf("%s:%d: testcase %q: step %d asserts %s before the first call",
				ts.Filepath, dryRunLine(ts.Filepath, tc, i, ""), tc.Name, i+1, must)
		}
	}
	return errors.Errorf("%s:%d: testcase %q: testcase has no call step", ts.Filepath, tc.SourceLine, tc.Name)
}

// dryRunMustOf is the first Must… operator of the assertions of a step, "" when there is none.
func dryRunMustOf(assertions interface{}) string {
	list, ok := assertions.([]interface{})
	if !ok {
		return ""
	}
	for _, a := range list {
		text, ok := a.(string)
		if !ok {
			raw, err := json.Marshal(a)
			if err != nil {
				continue
			}
			text = string(raw)
		}
		if m := dryRunMust.FindStringSubmatch(text); m != nil {
			return m[1]
		}
	}
	return ""
}

func dryRunHasDefault(expr string) bool {
	for _, w := range strings.FieldsFunc(expr, func(r rune) bool { return r == ' ' || r == '|' || r == '(' || r == ')' }) {
		for _, h := range dryRunNoCheck {
			if w == h {
				return true
			}
		}
	}
	return false
}

// dryRunKnown: the reference or one of its parents is set (.a.b of a var a holding a map), or a var under it is.
func dryRunKnown(ref string, known map[string]bool) bool {
	if strings.HasPrefix(ref, "venom.") || known[ref] {
		return true
	}
	for k := range known {
		if strings.HasPrefix(ref, k+".") || strings.HasPrefix(k, ref+".") {
			return true
		}
	}
	return false
}

// dryRunErr names the file and line of a step: the first line from the step down that holds the reference, else the
// line of the step, else of the testcase.
func dryRunErr(ts *TestSuite, tc *TestCase, step int, ref, msg string) error {
	return errors.Errorf("%s:%d: testcase %q step %d: %s", ts.Filepath, dryRunLine(ts.Filepath, tc, step, ref), tc.Name, step+1, msg)
}

func dryRunLine(file string, tc *TestCase, step int, ref string) int {
	line := tc.SourceLine
	if step >= 0 && step < len(tc.StepSourceLines) && tc.StepSourceLines[step] > 0 {
		line = tc.StepSourceLines[step]
	}
	if ref == "" || line <= 0 {
		return line
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return line
	}
	lines := strings.Split(string(content), "\n")
	for n := line - 1; n < len(lines); n++ {
		if strings.Contains(lines[n], "."+ref) {
			return n + 1
		}
	}
	return line
}

// fileLineError names a YAML error of a run or a dry run by <file>:<line> when its message carries the line.
func fileLineError(file string, err error) error {
	if m := dryRunYAMLLine.FindStringSubmatch(err.Error()); m != nil {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil {
			return errors.Errorf("%s:%d: %v", file, n, err)
		}
	}
	return errors.Errorf("%s: %v", file, err)
}

// dryRunUserExecutors resolves the step types of every user executor file to an executor; a file that does not parse
// before interpolation is checked at run time.
func (v *Venom) dryRunUserExecutors() error {
	names := make([]string, 0, len(v.executorsUser))
	for name := range v.executorsUser {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ux, ok := v.executorsUser[name].(UserExecutor)
		if !ok {
			continue
		}
		var doc yamlv3.Node
		if yamlv3.Unmarshal(ux.Raw, &doc) != nil || len(doc.Content) == 0 {
			continue
		}
		steps := yamlMapValue(doc.Content[0], "steps")
		if steps == nil || steps.Kind != yamlv3.SequenceNode {
			continue
		}
		for i, s := range steps.Content {
			t := yamlMapValue(s, "type")
			if t == nil || strings.Contains(t.Value, "{{") || v.executorKnown(t.Value) {
				continue
			}
			return errors.Errorf("%s:%d: executor %q step %d: unknown executor %q", ux.Filename, t.Line, name, i+1, t.Value)
		}
	}
	return nil
}

// DryRunSummary counts what a dry run loaded: testsuites, testcases, steps and user executors.
func (v *Venom) DryRunSummary() string {
	var testcases, steps int
	for _, ts := range v.Tests.TestSuites {
		testcases += len(ts.TestCases)
		for _, tc := range ts.TestCases {
			steps += len(tc.RawTestSteps)
		}
	}
	return fmt.Sprintf("%d testsuites, %d testcases, %d steps, %d user executors loaded; every executor resolved, every var set",
		len(v.Tests.TestSuites), testcases, steps, len(v.executorsUser))
}

func (v *Venom) executorKnown(name string) bool {
	if _, ok := v.executorsBuiltin[name]; ok {
		return true
	}
	if _, ok := v.executorsUser[name]; ok {
		return true
	}
	_, ok := v.executorsPlugin[name]
	return ok
}

func yamlMapValue(n *yamlv3.Node, key string) *yamlv3.Node {
	if n == nil || n.Kind != yamlv3.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
