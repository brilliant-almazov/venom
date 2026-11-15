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
	file := filepath.Join(dir, protocolFileName(tc.Name)+".md")
	if err := os.WriteFile(file, []byte(HideSensitive(ctx, protocolText(ts, tc))), 0o644); err != nil {
		Error(ctx, "unable to write protocol %s: %v", file, err)
	}
}

// protocolUnsafe are the runs of characters a file name cannot carry or that split it badly.
var protocolUnsafe = regexp.MustCompile(`[\s/\\:*?"<>|]+`)

// protocolFileName keeps the testcase name as it is, case and row id first (GW-B0005 …, S-001 …), so a reader finds the
// protocol by the row id; separators and characters a file name cannot carry become "-", the length stays under 120 bytes.
func protocolFileName(name string) string {
	n := strings.Trim(protocolUnsafe.ReplaceAllString(name, "-"), "-.")
	for len(n) > 120 {
		r := []rune(n)
		n = string(r[:len(r)-1])
	}
	if n == "" {
		return "testcase"
	}
	return n
}

func protocolText(ts *TestSuite, tc *TestCase) string {
	info := protocolInfo(tc)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nФайл: `%s`\n\n", tc.Name, ts.Filepath)

	b.WriteString("## Правильное поведение\n\n")
	protocolKeys(&b, info, "happens", "expect", "right", "norm")

	b.WriteString("\n## Что неправильно\n\n")
	protocolKeys(&b, info, "wrong")

	rows := protocolPresenceRows(tc)
	call, n := protocolLastCall(tc, rows)
	b.WriteString("\n## Наличие данных\n\n")
	b.WriteString(protocolPresenceTable(tc, rows, call))

	b.WriteString("\n## Факт\n\n")
	protocolKeys(&b, info, "point", "call")
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

// protocolPresence reads an info line of a precondition step, with or without a key in front ("expect: …"):
// «данные есть: <entity> · <key>» or «данных нет: <entity> · <key>».
var protocolPresence = regexp.MustCompile(`(?:^|:\s*)(данные есть|данных нет)\s*:\s*(.+?)\s*·\s*(.+?)(?:\s+\([^()]+:\d+\))?\s*$`)

// protocolPreconditionLine reads the precondition line of the scenario template (scenarios/TEMPLATE.*.yml of
// cgw-qa-scenarios): "precondition: <entity> <key> есть|нет — <what is read>".
var protocolPreconditionLine = regexp.MustCompile(`^\s*precondition\s*:\s*(\S+)\s+(.+?)\s+(есть|нет)(?:\s+—.*?)?(?:\s+\([^()]+:\d+\))?\s*$`)

type protocolPresenceRow struct {
	entity, key, presence string
	step                  int
}

// protocolPresenceRows are the precondition rows of the testcase, one per precondition info line.
func protocolPresenceRows(tc *TestCase) []protocolPresenceRow {
	var rows []protocolPresenceRow
	for i, r := range tc.TestStepResults {
		for _, l := range r.ComputedInfo {
			if m := protocolPresence.FindStringSubmatch(l); m != nil {
				rows = append(rows, protocolPresenceRow{entity: m[2], key: m[3], presence: m[1], step: i})
			} else if m := protocolPreconditionLine.FindStringSubmatch(l); m != nil {
				presence := "данные есть"
				if m[3] == "нет" {
					presence = "данных нет"
				}
				rows = append(rows, protocolPresenceRow{entity: m[1], key: m[2], presence: presence, step: i})
			}
		}
	}
	return rows
}

// protocolPreconditionHeld is false when the precondition read or a check after it failed: the steps from the read up
// to the next call (a step with a trace_id and a code), as the template checks the read in separate steps after its
// trace_save.
func protocolPreconditionHeld(tc *TestCase, step int) bool {
	for i := step; i < len(tc.TestStepResults); i++ {
		r := &tc.TestStepResults[i]
		if i > step && protocolIsCall(r) {
			break
		}
		if r.Status == StatusFail {
			return false
		}
	}
	return true
}

func protocolIsCall(r *TestStepResult) bool {
	_, traced := r.ComputedVars["result.trace_id"]
	return traced && protocolHasCode(r)
}

// protocolPresenceTable is the section «Наличие данных»: entity, key, the precondition request, presence with ✅ when
// the assertions of that read held and ❌ when they did not, its trace, and whether the tested call sent that key.
func protocolPresenceTable(tc *TestCase, rows []protocolPresenceRow, call *TestStepResult) string {
	if len(rows) == 0 {
		return "предусловия нет\n"
	}
	var b strings.Builder
	b.WriteString("| сущность | ключ | запрос | есть/нет | trace id | ключ в проверяемом вызове |\n| -- | -- | -- | -- | -- | -- |\n")
	for _, row := range rows {
		r := &tc.TestStepResults[row.step]
		mark := "✅"
		if !protocolPreconditionHeld(tc, row.step) {
			mark = "❌ не подтверждено"
		}
		trace := "—"
		if id := fmt.Sprint(r.ComputedVars["result.trace_id"]); r.ComputedVars["result.trace_id"] != nil && id != "" {
			trace = "`" + id + "`"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s %s | %s | %s |\n", row.entity, row.key, protocolRequest(r), row.presence, mark, trace,
			protocolSentKey(row.key, call, r))
	}
	return b.String()
}

// protocolSentKey says whether the tested call carries the value of the key, so a call that looked for something else
// is seen next to the row.
func protocolSentKey(key string, call, precondition *TestStepResult) string {
	if call == nil || call == precondition {
		return "проверяемого вызова нет"
	}
	value := key
	if i := strings.Index(key, "="); i >= 0 {
		value = key[i+1:]
	}
	req := protocolStep(call)
	delete(req, "info")
	delete(req, "assertions")
	delete(req, "vars")
	if value != "" && strings.Contains(fmt.Sprint(req), value) {
		return fmt.Sprintf("`%s` ✅ отправлен: %s", value, protocolRequest(call))
	}
	return fmt.Sprintf("`%s` ❌ не отправлен: %s", value, protocolRequest(call))
}

// protocolStep is the step after interpolation; Interpolated holds it as YAML bytes (process_testcase.go).
func protocolStep(r *TestStepResult) map[string]interface{} {
	m := map[string]interface{}{}
	if raw, ok := r.Interpolated.([]byte); ok {
		_ = yaml.Unmarshal(raw, &m)
	}
	return m
}

// protocolRequest names the request of a step: its «call:» info line, else method and url (http), else url, service and
// method (grpc), else the step type.
func protocolRequest(r *TestStepResult) string {
	for _, l := range r.ComputedInfo {
		if m := protocolInfoLine.FindStringSubmatch(l); m != nil && m[1] == "call" {
			return "`" + m[2] + "`"
		}
	}
	m := protocolStep(r)
	str := func(k string) string { s, _ := m[k].(string); return s }
	switch {
	case str("url") != "" && str("method") != "" && str("service") == "":
		return fmt.Sprintf("`%s %s`", str("method"), str("url"))
	case str("service") != "" || str("method") != "":
		return fmt.Sprintf("`grpc %s %s/%s`", str("url"), str("service"), str("method"))
	}
	return "`" + str("type") + "`"
}

// protocolLastCall is the call of the testcase, a passed precondition read left out: a step result with a trace_id and a code — result.statuscode (http) or
// result.code (grpc and the user executors that pass a call code on). The last failed call wins, then the last call;
// trace_save (a trace_id, no code) and check steps (a code, no trace_id) are not calls. With no such step the last step
// with a code is taken.
func protocolLastCall(tc *TestCase, rows []protocolPresenceRow) (*TestStepResult, int) {
	precondition := map[int]bool{}
	for _, row := range rows {
		precondition[row.step] = protocolPreconditionHeld(tc, row.step)
	}
	traced, coded := -1, -1
	for i := len(tc.TestStepResults) - 1; i >= 0; i-- {
		r := &tc.TestStepResults[i]
		if held, ok := precondition[i]; !protocolHasCode(r) || (ok && held) {
			continue
		}
		if _, ok := r.ComputedVars["result.trace_id"]; ok {
			if r.Status == StatusFail {
				return r, i
			}
			if traced < 0 {
				traced = i
			}
		}
		if coded < 0 {
			coded = i
		}
	}
	for _, i := range []int{traced, coded} {
		if i >= 0 {
			return &tc.TestStepResults[i], i
		}
	}
	return nil, -1
}

func protocolHasCode(r *TestStepResult) bool {
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
		m := protocolStep(&r)
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
