package venom

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callExecutor answers as an http call: statuscode from "code", a body and a trace_id. Result is named as the result
// of a real executor, so Venom dumps it under result.*.
type callExecutor struct{}

type Result struct {
	StatusCode int    `json:"statuscode"`
	Body       string `json:"body"`
	TraceID    string `json:"trace_id"`
}

func (callExecutor) Run(_ context.Context, step TestStep) (interface{}, error) {
	code, _ := step.IntValue("code")
	return Result{StatusCode: code, Body: fmt.Sprintf(`{"code":%d}`, code), TraceID: "0af7651916cd43dd8448eb211c80319c"}, nil
}

// checkExecutor answers as a check step of the template (exec echo of a var): systemout and code, no trace_id.
type checkExecutor struct{}

func (checkExecutor) Run(_ context.Context, step TestStep) (interface{}, error) {
	out, _ := step.StringValue("out")
	return map[string]interface{}{"systemout": out}, nil
}

func runProtocolSuite(t *testing.T, suite string, stopOnFailure bool) string {
	t.Helper()
	dir := t.TempDir()
	traces := filepath.Join(dir, "traces")
	require.NoError(t, os.MkdirAll(traces, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(traces, "0af7651916cd43dd8448eb211c80319c.md"), []byte("svc-billing ✅ GetAccount"), 0o644))
	file := filepath.Join(dir, "suite.yml")
	require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf(suite, traces)), 0o644))

	testLoggerOnce.Do(func() { InitTestLogger(t) })
	v := New()
	v.RegisterExecutorBuiltin("caller", callExecutor{})
	v.RegisterExecutorBuiltin("checker", checkExecutor{})
	v.OutputDir = dir
	v.ProtocolDir = filepath.Join(dir, "protocols")
	v.StopOnFailure = stopOnFailure
	var out bytes.Buffer
	v.PrintFunc = func(format string, a ...interface{}) (int, error) { return fmt.Fprintf(&out, format, a...) }
	ctx := context.Background()
	require.NoError(t, v.Parse(ctx, []string{file}))
	_ = v.Process(ctx, []string{file})
	return v.ProtocolDir
}

func readProtocol(t *testing.T, dir, testcase string) string {
	t.Helper()
	btes, err := os.ReadFile(filepath.Join(dir, "suite", testcase+".md"))
	require.NoError(t, err, "protocol of %s", testcase)
	return string(btes)
}

const testcaseInfo = `  info:
  - "expect: the account is read"
  - "right: statuscode 200"
  - "wrong: any other statuscode"
  - "norm: cgw-billing: docs/api.md:12"
`

func TestProtocolGreen(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: green
`+testcaseInfo+`  steps:
  - type: caller
    code: 200
    trace_dir: %s
    info:
    - "call: GET /v1/billing/accounts/1"
    assertions:
    - result.statuscode ShouldEqual 200
`, false)
	p := readProtocol(t, dir, "green")
	for _, want := range []string{"## Правильное поведение", "- expect: the account is read", "- right: statuscode 200", "- norm: cgw-billing: docs/api.md:12",
		"## Что неправильно", "- wrong: any other statuscode", "## Факт", "- call: GET /v1/billing/accounts/1", "- statuscode: 200",
		"## Трасса", "svc-billing ✅ GetAccount", "## Вердикт", "✅ пройден"} {
		assert.Contains(t, p, want)
	}
}

func TestProtocolMustFailed(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: must
`+testcaseInfo+`  steps:
  - type: caller
    code: 500
    trace_dir: %s
    assertions:
    - result.statuscode MustEqual 200
  - type: caller
    code: 200
`, false)
	p := readProtocol(t, dir, "must")
	assert.Contains(t, p, "❌ не пройден")
	assert.Contains(t, p, "result.statuscode MustEqual 200")
	assert.Contains(t, p, "- statuscode: 500")
	assert.Contains(t, p, "svc-billing ✅ GetAccount")
}

func TestProtocolStopOnFailure(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: first
`+testcaseInfo+`  steps:
  - type: caller
    code: 404
    trace_dir: %s
    assertions:
    - result.statuscode ShouldEqual 200
- name: second
  steps:
  - type: caller
    code: 200
`, true)
	first := readProtocol(t, dir, "first")
	assert.Contains(t, first, "❌ не пройден")
	assert.Contains(t, first, "result.statuscode ShouldEqual 200")
	second := readProtocol(t, dir, "second")
	assert.Contains(t, second, "⏭ пропущен")
	assert.Contains(t, second, "stop-on-failure")
}

func TestProtocolShouldFailedVarsSkipped(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: should
`+testcaseInfo+`  steps:
  - type: caller
    code: 422
    trace_dir: %s
    vars:
      got:
        from: result.body
    assertions:
    - result.statuscode ShouldEqual 200
  - type: caller
    code: 200
    info:
    - "fact: got={{.got}}"
`, false)
	p := readProtocol(t, dir, "should")
	assert.Contains(t, p, "❌ не пройден")
	assert.Contains(t, p, "result.statuscode ShouldEqual 200")
	assert.Contains(t, p, `{"code":422}`, "the failed call is the fact even though its vars were skipped")
	assert.Contains(t, p, "svc-billing ✅ GetAccount")
}

func TestProtocolTemplateShape(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: template
`+testcaseInfo+`  steps:
  - type: caller
    code: 404
    trace_dir: %s
    vars:
      call_status:
        from: result.statuscode
  - type: checker
    out: "{{.call_status}}"
    assertions:
    - systemout ShouldEqual 200
`, false)
	p := readProtocol(t, dir, "template")
	assert.Contains(t, p, "❌ не пройден")
	assert.Contains(t, p, "systemout ShouldEqual 200")
	assert.Contains(t, p, "- statuscode: 404", "the call is the fact, not the check step after it")
	assert.Contains(t, p, "svc-billing ✅ GetAccount")
}
