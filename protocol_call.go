package venom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// protocolHex32 is a W3C trace id; protocolBodyTraceID finds the meta.trace_id of a gateway answer in a raw body.
var (
	protocolHex32       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	protocolBodyTraceID = regexp.MustCompile(`"trace_id"\s*:\s*"([0-9a-f]{32})"`)
)

// protocolTraceKeys are the result keys a call carries its trace id under: the executor's own trace_id (grpc, http with
// trace:, user executors that pass one on), then meta.trace_id of a JSON answer, of an http call and of an exec-wrapped
// call that prints the answer.
var protocolTraceKeys = []string{"result.trace_id", "result.bodyjson.meta.trace_id", "result.systemoutjson.meta.trace_id"}

// protocolTraceID is the trace id of a step result: protocolTraceKeys, then the X-Trace-Id answer header (any case),
// then a "trace_id" of the raw answer body. Gateway calls that set no trace: on the http step return the id only in the
// header and in meta.trace_id (run 15-5: 1 169 of 1 686 protocols said «трассы нет» though the gateway returned one).
func protocolTraceID(r *TestStepResult) string {
	if r == nil {
		return ""
	}
	for _, k := range protocolTraceKeys {
		if id := fmt.Sprint(r.ComputedVars[k]); r.ComputedVars[k] != nil && protocolHex32.MatchString(id) {
			return id
		}
	}
	for k, val := range r.ComputedVars {
		if strings.EqualFold(k, "result.headers.x-trace-id") && protocolHex32.MatchString(fmt.Sprint(val)) {
			return fmt.Sprint(val)
		}
	}
	if !protocolHasCode(r) {
		return ""
	}
	for _, k := range []string{"result.body", "result.systemout"} {
		if m := protocolBodyTraceID.FindStringSubmatch(fmt.Sprint(r.ComputedVars[k])); r.ComputedVars[k] != nil && m != nil {
			return m[1]
		}
	}
	return ""
}

// protocolTraced: the step is able to carry a trace id (the key is there, even empty) or carries one.
func protocolTraced(r *TestStepResult) bool {
	if _, ok := r.ComputedVars["result.trace_id"]; ok {
		return true
	}
	return protocolTraceID(r) != ""
}

// protocolTreeFile is the saved span tree <dir>/<trace_id>.md in one of the trace_dir and traces the steps were given,
// and every path tried.
func protocolTreeFile(tc *TestCase, id string) (string, []string) {
	var tried []string
	for _, d := range protocolTraceDirs(tc) {
		f := filepath.Join(d, id+".md")
		tried = append(tried, f)
		if _, err := os.Stat(f); err == nil {
			return f, tried
		}
	}
	return "", tried
}

// traceGate fails a testcase whose call returned a trace id while no span tree of that id was saved: a protocol without
// the trace is not accepted (owner 2026-11-15: «Я сказал только с трейсами протоколы»).
func traceGate(tc *TestCase) {
	if tc.Status == StatusSkip {
		return
	}
	call, _ := protocolLastCall(tc, protocolPresenceRows(tc))
	id := protocolTraceID(call)
	if id == "" {
		return
	}
	if f, tried := protocolTreeFile(tc, id); f == "" {
		where := strings.Join(tried, ", ")
		if where == "" {
			where = "no step was given trace_dir or traces"
		}
		call.Errors = append(call.Errors, Failure{Value: fmt.Sprintf("trace %s returned by the call has no saved span tree: %s", id, where)})
		call.Status = StatusFail
		tc.Status = StatusFail
	}
}

// protocolRequestBlock is what the call sent: method, full url, headers (Authorization masked) and the body verbatim,
// JSON one key per line; a step that is not http shows its call fields from the interpolated step.
func protocolRequestBlock(r *TestStepResult) string {
	var b strings.Builder
	if method, ok := r.ComputedVars["result.request.method"]; ok {
		fmt.Fprintf(&b, protocolMethodLine+"%v\n- url: %v\n", method, r.ComputedVars["result.request.url"])
		b.WriteString(protocolHeaders(r, "result.request.header.", nil))
		b.WriteString(protocolBodyBlock("тело", fmt.Sprint(r.ComputedVars["result.request.body"])))
		return b.String()
	}
	step := protocolStep(r)
	fmt.Fprintf(&b, "- тип шага: %v\n", step["type"])
	for _, k := range []string{"method", "url", "gateway", "path", "service", "addr", "role"} {
		if v, ok := step[k]; ok && fmt.Sprint(v) != "" {
			fmt.Fprintf(&b, "- %s: %v\n", k, v)
		}
	}
	for _, k := range []string{"headers", "metadata"} {
		if h, ok := step[k].(map[string]interface{}); ok && len(h) > 0 {
			fmt.Fprintf(&b, "- %s:\n", k)
			for _, name := range protocolSorted(h) {
				fmt.Fprintf(&b, "  - %s: %s\n", name, protocolMask(name, fmt.Sprint(h[name])))
			}
		}
	}
	for _, k := range []string{"body", "data_json", "data", "query", "script"} {
		v, ok := step[k]
		if !ok || fmt.Sprint(v) == "" {
			continue
		}
		text := fmt.Sprint(v)
		if _, isMap := v.(map[string]interface{}); isMap {
			if raw, err := json.Marshal(v); err == nil {
				text = string(raw)
			}
		}
		b.WriteString(protocolBodyBlock(k, text))
	}
	return b.String()
}

// protocolResponseBlock is what the call answered: the code, the X-Trace-Id, Traceparent, X-Request-Id and X-Build headers and the
// body verbatim, JSON one key per line.
func protocolResponseBlock(r *TestStepResult) string {
	var b strings.Builder
	for _, k := range []string{"result.statuscode", "result.code", "result.message"} {
		if val, ok := r.ComputedVars[k]; ok && fmt.Sprint(val) != "" {
			fmt.Fprintf(&b, "- %s: %v\n", strings.TrimPrefix(k, "result."), val)
		}
	}
	if id := protocolTraceID(r); id != "" {
		fmt.Fprintf(&b, "- trace_id: %s\n", id)
	}
	b.WriteString(protocolHeaders(r, "result.headers.", []string{"x-trace-id", "traceparent", "x-request-id", "x-build"}))
	b.WriteString(protocolBodyBlock("тело", protocolBody(r)))
	return b.String()
}

// protocolAfter is the state after the call: every later call of the testcase — the read-back after a write — with
// its request and answer; none is said so.
func protocolAfter(tc *TestCase, call *TestStepResult, n int) string {
	if call == nil {
		return "вызова в testcase нет\n"
	}
	var b strings.Builder
	for i := n + 1; i < len(tc.TestStepResults); i++ {
		r := &tc.TestStepResults[i]
		if !protocolIsCall(r) {
			continue
		}
		fmt.Fprintf(&b, protocolAfterStep+"\n\n", i+1, r.Name, protocolRequest(r))
		b.WriteString(protocolRequestBlock(r))
		b.WriteString(protocolResponseBlock(r))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "чтения назад после вызова в testcase нет\n"
	}
	return b.String()
}

// protocolHeaders lists the headers under prefix: only the names in only (any case) when given, else all; a header
// value dumped as a list is taken from its first element.
func protocolHeaders(r *TestStepResult, prefix string, only []string) string {
	values := map[string]string{}
	for k, val := range r.ComputedVars {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		name := strings.TrimPrefix(k, prefix)
		if strings.Contains(name, ".") || strings.HasPrefix(name, "__") {
			continue
		}
		s := fmt.Sprint(val)
		if first, ok := r.ComputedVars[k+"."+name+"0"]; ok {
			s = fmt.Sprint(first)
		}
		values[name] = s
	}
	var names []string
	for name := range values {
		if only == nil {
			names = append(names, name)
			continue
		}
		for _, o := range only {
			if strings.EqualFold(o, name) {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("- заголовки:\n")
	for _, name := range names {
		fmt.Fprintf(&b, "  - %s: %s\n", name, protocolMask(name, values[name]))
	}
	return b.String()
}

// protocolMask hides the token of an Authorization header, its scheme and length kept.
func protocolMask(name, value string) string {
	if !strings.EqualFold(name, "authorization") || value == "" {
		return value
	}
	scheme, token := "", value
	if i := strings.Index(value, " "); i >= 0 {
		scheme, token = value[:i+1], strings.TrimSpace(value[i+1:])
	}
	return fmt.Sprintf("%s"+protocolMasked, scheme, len(token))
}

// protocolBodyBlock prints a body verbatim in a code block, a JSON body one key per line, cut at protocolBodyMax.
func protocolBodyBlock(title, body string) string {
	if body == "" || body == "<nil>" {
		return fmt.Sprintf("- %s: пусто\n", title)
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(strings.TrimSpace(body)), "", "  ") == nil {
		body = pretty.String()
	}
	if len(body) > protocolBodyMax {
		body = body[:protocolBodyMax] + fmt.Sprintf("\n… обрезано, всего %d байт", len(body))
	}
	return fmt.Sprintf("- %s:\n\n```\n%s\n```\n", title, body)
}

func protocolSorted(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
