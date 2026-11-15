package venom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gosimple/slug"
	"github.com/rockbears/yaml"

	"github.com/ovh/venom/interpolate"
)

// protocolBodyMax caps the body of the call printed under «Факт»; the rest is cut with a note.
const protocolBodyMax = 4000

// protocolInfoLine reads an info line "key: value"; the " (file:line)" suffix Venom adds to step info is dropped.
var protocolInfoLine = regexp.MustCompile(`^\s*([a-z_]+)\s*:\s*(.*?)(?:\s+\([^()]+:\d+\))?\s*$`)

// writeProtocol writes <ProtocolDir>/<testsuite>/<testcase>.md for one testcase, whatever its outcome: the expected and
// the wrong behaviour from the info lines of the testcase and its steps, the result of its last call, the span tree of
// that call and the verdict with every failed assertion. Stock Venom keeps none of it in a file per testcase: a step
// after a failed Must assertion or under --stop-on-failure never runs, and a failed step sets none of its vars, so a
// suite step cannot write it.
func (v *Venom) writeProtocol(ctx context.Context, ts *TestSuite, tc *TestCase) {
	if v.ProtocolDir == "" {
		return
	}
	dir := filepath.Join(v.ProtocolDir, slug.Make(ts.ShortName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Error(ctx, "unable to create protocol dir %s: %v", dir, err)
		return
	}
	file := filepath.Join(dir, slug.Make(tc.Name)+".md")
	if err := os.WriteFile(file, []byte(HideSensitive(ctx, protocolText(ts, tc))), 0o644); err != nil {
		Error(ctx, "unable to write protocol %s: %v", file, err)
	}
}

func protocolText(ts *TestSuite, tc *TestCase) string {
	info := protocolInfo(tc)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nФайл: `%s`\n\n", tc.Name, ts.Filepath)

	b.WriteString("## Правильное поведение\n\n")
	protocolKeys(&b, info, "happens", "expect", "right", "norm")

	b.WriteString("\n## Что неправильно\n\n")
	protocolKeys(&b, info, "wrong")

	b.WriteString("\n## Факт\n\n")
	protocolKeys(&b, info, "point", "call")
	call, n := protocolLastCall(tc)
	if call == nil {
		b.WriteString("- результат вызова: вызова в testcase нет\n")
	} else {
		fmt.Fprintf(&b, "- шаг: %d «%s»\n", n+1, call.Name)
		for _, k := range []string{"result.statuscode", "result.code", "result.message"} {
			if val, ok := call.ComputedVars[k]; ok && fmt.Sprint(val) != "" {
				fmt.Fprintf(&b, "- %s: %v\n", strings.TrimPrefix(k, "result."), val)
			}
		}
		body := protocolBody(call)
		if len(body) > protocolBodyMax {
			body = body[:protocolBodyMax] + fmt.Sprintf("\n… обрезано, всего %d байт", len(body))
		}
		fmt.Fprintf(&b, "- тело:\n\n```\n%s\n```\n", body)
	}
	for _, f := range info["fact"] {
		fmt.Fprintf(&b, "- fact: %s\n", f)
	}

	b.WriteString("\n## Трасса\n\n")
	b.WriteString(protocolTrace(tc, call))

	b.WriteString("\n## Вердикт\n\n")
	b.WriteString(protocolVerdict(tc))
	return b.String()
}

// protocolInfo collects the "key: value" info lines of the testcase (interpolated on its vars) and of every step result.
func protocolInfo(tc *TestCase) map[string][]string {
	vars := tc.Vars.Clone()
	vars.AddAll(tc.computedVars)
	flat := make(map[string]string, len(vars))
	for k, val := range vars {
		flat[k] = fmt.Sprint(val)
	}
	lines := make([]string, 0, len(tc.Info))
	for _, l := range tc.Info {
		if s, err := interpolate.Do(l, flat); err == nil {
			l = s
		}
		lines = append(lines, l)
	}
	for _, r := range tc.TestStepResults {
		lines = append(lines, r.ComputedInfo...)
	}
	info := map[string][]string{}
	seen := map[string]bool{}
	for _, l := range lines {
		m := protocolInfoLine.FindStringSubmatch(l)
		if m == nil || seen[m[1]+"\x00"+m[2]] {
			continue
		}
		seen[m[1]+"\x00"+m[2]] = true
		info[m[1]] = append(info[m[1]], m[2])
	}
	return info
}

func protocolKeys(b *strings.Builder, info map[string][]string, keys ...string) {
	for _, k := range keys {
		vals := info[k]
		if len(vals) == 0 {
			fmt.Fprintf(b, "- %s: не объявлено в info\n", k)
			continue
		}
		for _, val := range vals {
			fmt.Fprintf(b, "- %s: %s\n", k, val)
		}
	}
}

// protocolLastCall is the last failed step result that answered with a code, or the last such step when none failed:
// result.statuscode (http) or result.code (grpc, exec and the user executors that pass a call code on).
func protocolLastCall(tc *TestCase) (*TestStepResult, int) {
	last := -1
	for i := len(tc.TestStepResults) - 1; i >= 0; i-- {
		r := &tc.TestStepResults[i]
		if !protocolIsCall(r) {
			continue
		}
		if r.Status == StatusFail {
			return r, i
		}
		if last < 0 {
			last = i
		}
	}
	if last < 0 {
		return nil, -1
	}
	return &tc.TestStepResults[last], last
}

func protocolIsCall(r *TestStepResult) bool {
	for _, k := range []string{"result.statuscode", "result.code"} {
		if _, ok := r.ComputedVars[k]; ok {
			return true
		}
	}
	return false
}

func protocolBody(r *TestStepResult) string {
	for _, k := range []string{"result.body", "result.systemout"} {
		if val, ok := r.ComputedVars[k]; ok && fmt.Sprint(val) != "" {
			return fmt.Sprint(val)
		}
	}
	return ""
}

// protocolTrace prints the span tree <trace_dir>/<trace_id>.md of the last call, looked up in every trace_dir and
// traces the steps of the testcase were given.
func protocolTrace(tc *TestCase, call *TestStepResult) string {
	if call == nil {
		return "- трассы нет: вызова в testcase нет\n"
	}
	id := fmt.Sprint(call.ComputedVars["result.trace_id"])
	if call.ComputedVars["result.trace_id"] == nil || id == "" {
		return "- трассы нет: вызов не вернул trace_id\n"
	}
	var tried []string
	for _, d := range protocolTraceDirs(tc) {
		f := filepath.Join(d, id+".md")
		tried = append(tried, f)
		if btes, err := os.ReadFile(f); err == nil {
			return fmt.Sprintf("- trace_id: %s\n- файл: `%s`\n\n%s\n", id, f, strings.TrimRight(string(btes), "\n"))
		}
	}
	if len(tried) == 0 {
		return fmt.Sprintf("- trace_id: %s\n- дерева нет: ни один шаг не получил trace_dir или traces\n", id)
	}
	return fmt.Sprintf("- trace_id: %s\n- дерева нет: %s\n", id, strings.Join(tried, ", "))
}

func protocolTraceDirs(tc *TestCase) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, r := range tc.TestStepResults {
		// Interpolated holds the step after interpolation as YAML bytes (process_testcase.go)
		raw, ok := r.Interpolated.([]byte)
		if !ok {
			continue
		}
		var m map[string]interface{}
		if err := yaml.Unmarshal(raw, &m); err != nil {
			continue
		}
		for _, k := range []string{"trace_dir", "traces"} {
			d, ok := m[k].(string)
			if ok && d != "" && !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

func protocolVerdict(tc *TestCase) string {
	var b strings.Builder
	switch tc.Status {
	case StatusPass:
		b.WriteString("✅ пройден\n")
	case StatusSkip:
		b.WriteString("⏭ пропущен\n")
		for _, s := range tc.Skipped {
			fmt.Fprintf(&b, "- %s\n", s.Value)
		}
		for _, r := range tc.TestStepResults {
			for _, s := range r.Skipped {
				fmt.Fprintf(&b, "- шаг «%s»: %s\n", r.Name, s.Value)
			}
		}
	default:
		b.WriteString("❌ не пройден\n")
		for i, r := range tc.TestStepResults {
			for _, e := range r.Errors {
				fmt.Fprintf(&b, "- шаг %d «%s»: %s\n", i+1, r.Name, e.Value)
			}
		}
	}
	return b.String()
}
