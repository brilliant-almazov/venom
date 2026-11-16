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

// protocolBodyMax caps a body printed under Request, Response and State after; the rest is cut with a note.
const protocolBodyMax = 20000

// protocolInfoLine reads an info line "key: value"; the " (file:line)" suffix Venom adds to step info is dropped.
var protocolInfoLine = regexp.MustCompile(`^\s*([a-z_]+)\s*:\s*(.*?)(?:\s+\([^()]+:\d+\))?\s*$`)

// writeProtocol writes <ProtocolDir>/<testsuite>/<testcase>--<outcome>.md (protocolFile) for one testcase, whatever its outcome: the expected and
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
	file := filepath.Join(dir, protocolFile(tc))
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
	fmt.Fprintf(&b, "# %s\n\nFile: `%s`\n\n", tc.Name, ts.Filepath)

	b.WriteString(protocolHeadRight + "\n\n")
	protocolKeys(&b, info, "happens", "expect", "right", "norm")

	b.WriteString("\n" + protocolHeadWrong + "\n\n")
	protocolKeys(&b, info, "wrong")

	rows := protocolPresenceRows(tc)
	call, n := protocolLastCall(tc, rows)
	b.WriteString("\n" + protocolHeadBefore + "\n\n")
	b.WriteString(protocolBefore(tc, rows))
	b.WriteString("\n" + protocolHeadPresence + "\n\n")
	b.WriteString(protocolPresenceTable(tc, rows, call))

	b.WriteString("\n" + protocolHeadRequest + "\n\n")
	protocolKeys(&b, info, "point")
	b.WriteString(protocolWaits(tc))
	if call == nil {
		b.WriteString("- call: the testcase has no call\n")
	} else {
		fmt.Fprintf(&b, "- call: %s\n", protocolRequest(call))
		fmt.Fprintf(&b, protocolStepLine+"\n", n+1, call.Name)
		b.WriteString(protocolRequestBlock(call))
	}

	b.WriteString("\n" + protocolHeadAnswer + "\n\n")
	if call == nil {
		b.WriteString("- the testcase has no call\n")
	} else {
		b.WriteString(protocolResponseBlock(call))
	}
	for _, f := range info["fact"] {
		fmt.Fprintf(&b, "- fact: %s\n", f)
	}

	b.WriteString("\n" + protocolHeadAfter + "\n\n")
	b.WriteString(protocolAfter(tc, call, n))

	b.WriteString("\n" + protocolHeadTrace + "\n\n")
	b.WriteString(protocolTrace(tc, call))

	b.WriteString("\n" + protocolHeadVerdict + "\n\n")
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
			fmt.Fprintf(b, "- %s: not declared in info\n", k)
			continue
		}
		for _, val := range vals {
			fmt.Fprintf(b, "- %s: %s\n", k, val)
		}
	}
}

// protocolPreconditionLine reads the precondition line of a read step, the one form of the scenario template
// (scenarios/TEMPLATE.*.yml of cgw-qa-scenarios): "precondition: <entity> · <key> present|absent — <what is read>".
var protocolPreconditionLine = regexp.MustCompile(`^\s*precondition\s*:\s*(.+?)\s*·\s*(.+?)\s+(present|absent)(?:\s+—.*?)?(?:\s+\([^()]+:\d+\))?\s*$`)

type protocolPresenceRow struct {
	entity, key, presence string
	step                  int
}

// protocolPresenceRows are the precondition rows of the testcase, one per precondition line; the «data present: …» of an
// expect is the words of the scenario and makes no row.
func protocolPresenceRows(tc *TestCase) []protocolPresenceRow {
	var rows []protocolPresenceRow
	for i, r := range tc.TestStepResults {
		for _, l := range r.ComputedInfo {
			if m := protocolPreconditionLine.FindStringSubmatch(l); m != nil {
				presence := protocolDataPresent
				if m[3] == protocolWordAbsent {
					presence = protocolDataAbsent
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
	return protocolTraced(r) && protocolHasCode(r)
}

// protocolPresenceTable is the section State before: entity, key, the precondition request, presence with ✅ when
// the assertions of that read held and ❌ when they did not, its trace, and whether the tested call sent that key.
func protocolPresenceTable(tc *TestCase, rows []protocolPresenceRow, call *TestStepResult) string {
	if len(rows) == 0 {
		return protocolNoPrecondition + "\n"
	}
	var b strings.Builder
	b.WriteString(protocolPresenceHeader + "\n| -- | -- | -- | -- | -- | -- |\n")
	for _, row := range rows {
		r := &tc.TestStepResults[row.step]
		mark := "✅"
		if !protocolPreconditionHeld(tc, row.step) {
			mark = protocolNotConfirmed
		}
		trace := "—"
		if id := protocolTraceID(r); id != "" {
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
		return "no tested call"
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
		return fmt.Sprintf("`%s` "+protocolKeySent+": %s", value, protocolRequest(call))
	}
	return fmt.Sprintf("`%s` "+protocolKeyNotSent+": %s", value, protocolRequest(call))
}

// protocolStep is the step after interpolation; Interpolated holds it as YAML bytes (process_testcase.go).
func protocolStep(r *TestStepResult) map[string]interface{} {
	m := map[string]interface{}{}
	if raw, ok := r.Interpolated.([]byte); ok {
		_ = yaml.Unmarshal(raw, &m)
	}
	return m
}

// protocolRequest names the request of a step declaratively, from the step itself and never from the text of a script:
// the executor, then the method and full url of an http call, else the url, service and method of a grpc call, else
// the declared fields of the step (owner 2026-11-15: scenarios and protocols are declarative only).
func protocolRequest(r *TestStepResult) string {
	m := protocolStep(r)
	str := func(k string) string { return protocolField(m[k]) }
	if method, ok := protocolHTTPMethod(r); ok {
		return fmt.Sprintf("`%s %v %v`", str("type"), method, r.ComputedVars["result.request.url"])
	}
	if str("service") != "" {
		return fmt.Sprintf("`%s %s %s/%s`", str("type"), str("url"), str("service"), str("method"))
	}
	words := []string{str("type")}
	for _, k := range []string{"method", "url", "path"} {
		if v := str(k); v != "" {
			words = append(words, v)
		}
	}
	for _, k := range protocolDeclaredFields {
		if v := str(k); v != "" {
			words = append(words, k+"="+v)
		}
	}
	return "`" + strings.Join(words, " ") + "`"
}

// protocolDeclaredFields are the fields of a step that is not an http or grpc call, as the call line and the Request
// section print them: the stand utility (cli, seed, stand_status), the openapi operation, a trace save, a check.
var protocolDeclaredFields = []string{"action", "args", "env", "block", "role", "trace_id", "value"}

// protocolField prints a field of the step: a list as its items joined by a space.
func protocolField(v interface{}) string {
	if list, ok := v.([]interface{}); ok {
		words := make([]string, 0, len(list))
		for _, w := range list {
			words = append(words, fmt.Sprint(w))
		}
		return strings.Join(words, " ")
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// protocolWaits lists the wait steps of the testcase, each with its seconds and reason.
func protocolWaits(tc *TestCase) string {
	var b strings.Builder
	for i := range tc.TestStepResults {
		m := protocolStep(&tc.TestStepResults[i])
		if protocolField(m["type"]) != "wait" {
			continue
		}
		reason := protocolField(m["reason"])
		if reason == "" {
			reason = protocolNoReason
		}
		fmt.Fprintf(&b, protocolWaitLine+"\n", i+1, protocolField(m["seconds"]), reason)
	}
	return b.String()
}

// protocolLastCall is the call of the testcase, a passed precondition read left out, among the steps with a «call:» info
// line when the testcase has one: a step result with a trace_id and a code — result.statuscode (http) or
// result.code (grpc and the user executors that pass a call code on). The last failed call wins, then the last call;
// trace_save (a trace_id, no code) and check steps (a code, no trace_id) are not calls. With no such step the last step
// with a code is taken.
func protocolLastCall(tc *TestCase, rows []protocolPresenceRow) (*TestStepResult, int) {
	precondition := map[int]bool{}
	for _, row := range rows {
		precondition[row.step] = protocolPreconditionHeld(tc, row.step)
	}
	named := false
	for i := range tc.TestStepResults {
		if _, ok := precondition[i]; !ok && protocolNamesCall(&tc.TestStepResults[i]) {
			named = true
		}
	}
	traced, coded := -1, -1
	for i := len(tc.TestStepResults) - 1; i >= 0; i-- {
		r := &tc.TestStepResults[i]
		if held, ok := precondition[i]; !protocolHasCode(r) || (ok && held) || (named && !protocolNamesCall(r)) {
			continue
		}
		if protocolTraced(r) {
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

// protocolNamesCall: the step carries the «call:» info line, the line the template puts on the tested call only; a
// read-back after a write carries none, so with such a line the call is never a later read-back.
func protocolNamesCall(r *TestStepResult) bool {
	for _, l := range r.ComputedInfo {
		if m := protocolInfoLine.FindStringSubmatch(l); m != nil && m[1] == "call" {
			return true
		}
	}
	return false
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
		return "- " + protocolNoTrace + ": the testcase has no call\n"
	}
	id := protocolTraceID(call)
	if id == "" {
		return protocolStepTraces(tc)
	}
	return protocolTree(tc, id)
}

// protocolStepTraces prints, for a call that returned no trace id, the span tree of every other step of the testcase that
// did, under "- step N": a teardown whose stand step failed after its calls each returned a trace (run 16-2: 9 protocols
// with a trace id and no tree).
func protocolStepTraces(tc *TestCase) string {
	var b strings.Builder
	b.WriteString("- " + protocolNoTrace + ": the call returned no trace_id in its result, the X-Trace-Id header or meta.trace_id of the body\n")
	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		if id := protocolTraceID(r); id != "" && protocolHasCode(r) {
			fmt.Fprintf(&b, "\n"+protocolStepLine+"\n%s", i+1, r.Name, protocolTree(tc, id))
		}
	}
	return b.String()
}

// protocolTree prints the span tree <trace_dir>/<id>.md, or why there is none.
func protocolTree(tc *TestCase, id string) string {
	f, tried := protocolTreeFile(tc, id)
	if f != "" {
		btes, err := os.ReadFile(f)
		if err == nil {
			return fmt.Sprintf("- trace_id: %s\n- file: `%s`\n\n%s\n", id, f, strings.TrimRight(string(btes), "\n"))
		}
	}
	if len(tried) == 0 {
		return fmt.Sprintf("- trace_id: %s\n"+protocolNoTree+" no step was given trace_dir or traces\n", id)
	}
	return fmt.Sprintf("- trace_id: %s\n"+protocolNoTree+" %s\n", id, strings.Join(tried, ", "))
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
		b.WriteString(protocolPassed + "\n")
	case StatusSkip:
		b.WriteString(protocolSkipped + "\n")
		for _, s := range tc.Skipped {
			fmt.Fprintf(&b, "- %s\n", s.Value)
		}
		for _, r := range tc.TestStepResults {
			for _, s := range r.Skipped {
				fmt.Fprintf(&b, "- step \"%s\": %s\n", r.Name, s.Value)
			}
		}
	default:
		b.WriteString(protocolFailed + "\n")
		for i, r := range tc.TestStepResults {
			for _, e := range r.Errors {
				fmt.Fprintf(&b, "- step %d \"%s\": %s\n", i+1, r.Name, e.Value)
			}
		}
	}
	return b.String()
}
