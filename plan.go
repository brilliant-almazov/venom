package venom

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rockbears/yaml"
	"github.com/sirupsen/logrus"

	"github.com/ovh/venom/interpolate"
)

// planDepth is how deep the calls of a user executor that calls user executors are followed.
const planDepth = 4

// ReadPlan reads every testsuite file of the trees of path, at any depth, as written — no var filled, no step run,
// nothing touches the network — and the user executors of their lib folders (section 26): what a scheme with no run is
// drawn from (venom schemes). A file under a lib folder, or with no testcases, is no testsuite. With no logger set
// (InitLogger writes venom.log of a run) it logs warnings to stderr.
func (v *Venom) ReadPlan(ctx context.Context, path []string) error {
	var files []string

	for _, p := range path {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			p = filepath.Join(p, "**", "*.y*ml")
		}

		files = append(files, p)
	}

	filesPath, err := getFilesPath(files)
	if err != nil {
		return err
	}

	suites := make([]string, 0, len(filesPath))

	for _, f := range filesPath {
		if isTestsuiteFile(f) {
			suites = append(suites, f)
		}
	}

	if len(suites) == 0 {
		return fmt.Errorf("no testsuite file in %v", path)
	}

	v.AsWritten = true

	if logger == nil {
		l := logrus.New()
		l.SetLevel(logrus.WarnLevel)
		logger = logrus.NewEntry(l)
	}

	if err := v.readFiles(ctx, suites); err != nil {
		return err
	}

	return v.registerUserExecutors(ctx)
}

// isTestsuiteFile: the file is outside every lib folder and has testcases.
func isTestsuiteFile(file string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(file)), "/") {
		if part == "lib" {
			return false
		}
	}

	body, err := os.ReadFile(file)

	return err == nil && readPartialYML(body, "testcases") != ""
}

// PlanView is the view of a testcase with no run: its steps as the scenario writes them, with no mark and no reason, and
// under each step of a user executor the steps that executor makes with the inputs of the step, down to its own.
func (v *Venom) PlanView(ts *TestSuite, tc *TestCase) ProtocolView {
	view := PlannedView(ts, tc)

	for i := range view.Steps {
		view.Steps[i].Mark, view.Steps[i].Reason = "", ""
		view.Steps[i].Calls = v.callsOf(tc.RawTestSteps[i], planDepth)
	}

	return view
}

// callsOf are the steps of the user executor the step raw calls, its inputs filled from the step as written or from the
// defaults of the executor, as RunUserExecutor fills them; none when raw is no user executor step.
func (v *Venom) callsOf(raw json.RawMessage, depth int) []StepView {
	step := map[string]interface{}{}

	if depth == 0 || json.Unmarshal(raw, &step) != nil {
		return nil
	}

	ux, ok := v.executorsUser[protocolField(step["type"])].(UserExecutor)
	if !ok {
		return nil
	}

	var in, inner UserExecutor
	_ = yaml.Unmarshal(ux.RawInputs, &in)
	vars := H{}

	for k, def := range in.Input {
		val, ok := step[k]
		if !ok || val == nil || val == "" {
			val = def
		}

		vars.AddWithPrefix("input", k, val)
	}

	flat, err := DumpString(vars)
	if err != nil {
		return nil
	}

	text, err := interpolate.Do(string(ux.Raw), flat)
	if err != nil || yaml.Unmarshal(quoteTemplateExpressions([]byte(text)), &inner) != nil {
		return nil
	}

	calls := make([]StepView, 0, len(inner.TestSteps))

	for i, s := range inner.TestSteps {
		call := plannedStepOf(s, i+1, 0)
		call.Mark, call.Reason = "", ""
		call.Calls = v.callsOf(s, depth-1)
		calls = append(calls, call)
	}

	return calls
}
