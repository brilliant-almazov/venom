package venom

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callExecutor answers as an http call: statuscode from "code", a body and a trace_id. Result is named as the result
// of a real executor, so Venom dumps it under result.*.
type callExecutor struct{}

type Result struct {
	StatusCode int               `json:"statuscode"`
	Body       string            `json:"body"`
	TraceID    string            `json:"trace_id,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Request    *testRequest      `json:"request,omitempty"`
}

type testRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
	Body   string              `json:"body"`
}

// gatewayExecutor answers as a gateway call over the http executor without trace: the trace id only in the X-Trace-Id
// header and in meta.trace_id of the body, as the billing gateway suites of run 15-5 got it.
type gatewayExecutor struct{}

func (gatewayExecutor) Run(_ context.Context, step TestStep) (interface{}, error) {
	code, _ := step.IntValue("code")
	return Result{
		StatusCode: code,
		Body:       `{"errors":[{"code":"role_required"}],"meta":{"trace_id":"0af7651916cd43dd8448eb211c80319c"}}`,
		Headers:    map[string]string{"X-Trace-Id": "0af7651916cd43dd8448eb211c80319c", "X-Request-Id": "r-1", "X-Build": "dev-1", "Server": "nginx"},
		Request: &testRequest{Method: "POST", URL: "http://nginx-gateway/v1/billing/accounts",
			Header: map[string][]string{"Authorization": {"Bearer secret-token"}, "X-Idempotency-Key": {"NloBboqV"}},
			Body:   `{"tenant_id":"7"}`},
	}, nil
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
	v.RegisterExecutorBuiltin("gateway", gatewayExecutor{})
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
	btes, err := os.ReadFile(filepath.Join(dir, "suite", protocolFileName(testcase)+".md"))
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
		"## Что неправильно", "- wrong: any other statuscode", "## Запрос", "## Ответ", "## Состояние после вызова", "- call: GET /v1/billing/accounts/1", "- statuscode: 200",
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
	assert.Contains(t, p, "## Ответ\n\n- statuscode: 422", "the failed call is the fact even though its vars were skipped")
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

const presenceSuite = `name: suite
testcases:
- name: present
` + testcaseInfo + `  steps:
  - type: caller
    code: 200
    trace_dir: %[1]s
    info:
    - "call: GET /v1/billing/accounts/7"
    - "precondition: account · id=7 есть — чтение до проверяемого вызова"
    - "expect: данные есть: account · id=7 — счёт читается"
    assertions:
    - result.statuscode MustEqual 200
  - type: caller
    code: 200
    url: /v1/billing/accounts/7/entries
    info:
    - "expect: данные есть: account · id=7 — записи счёта читаются"
- name: absent
  steps:
  - type: caller
    code: 404
    info:
    - "call: GET /v1/billing/accounts/9223372036854775807"
    - "precondition: account · id=9223372036854775807 нет — чтение до проверяемого вызова"
    assertions:
    - result.statuscode MustEqual 404
  - type: caller
    code: 404
    url: /v1/billing/accounts/42
- name: failed
  steps:
  - type: caller
    code: 404
    info:
    - "precondition: account · id=7 есть — чтение до проверяемого вызова"
    assertions:
    - result.statuscode MustEqual 200
  - type: caller
    code: 200
    url: /v1/billing/accounts/7
- name: none
  steps:
  - type: caller
    code: 200
`

func TestProtocolPresence(t *testing.T) {
	dir := runProtocolSuite(t, presenceSuite, false)

	p := readProtocol(t, dir, "present")
	assert.Contains(t, p, "## Наличие данных\n\n| сущность | ключ | запрос | есть/нет | trace id | ключ в проверяемом вызове |")
	assert.Contains(t, p, "| account | `id=7` | `GET /v1/billing/accounts/7` | данные есть ✅ | `0af7651916cd43dd8448eb211c80319c` | `7` ✅ отправлен")
	assert.Less(t, strings.Index(p, "## Наличие данных"), strings.Index(p, "## Запрос"), "the table goes before the main call")
	assert.Equal(t, 1, strings.Count(p, "| account |"), "the expect «данные есть: …» of the call makes no row")

	a := readProtocol(t, dir, "absent")
	assert.Contains(t, a, "| account | `id=9223372036854775807` | `GET /v1/billing/accounts/9223372036854775807` | данных нет ✅ |")
	assert.Contains(t, a, "`9223372036854775807` ❌ не отправлен", "the tested call looked for another id")

	f := readProtocol(t, dir, "failed")
	assert.Contains(t, f, "данные есть ❌ не подтверждено")
	assert.Contains(t, f, "❌ не пройден")
	assert.Contains(t, f, "result.statuscode MustEqual 200", "the testcase fails at the precondition")
	assert.Contains(t, f, "- шаг: 1 «caller»", "the fact is the failed precondition read")

	n := readProtocol(t, dir, "none")
	assert.Contains(t, n, "## Наличие данных\n\nпредусловия нет\n")
	assert.NotContains(t, n, "| сущность |")
}

func TestProtocolFileNameKeepsRowID(t *testing.T) {
	assert.Equal(t, "GW-B0005-DELETE-v1-billing-accounts-{account_id}-без-ключа", protocolFileName("GW-B0005 DELETE /v1/billing/accounts/{account_id} без ключа"))
	assert.Equal(t, "S-001-CreateAccount", protocolFileName("S-001: CreateAccount"))
	assert.Equal(t, "testcase", protocolFileName(" / "))
}

func TestProtocolPresenceTemplateShape(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: held
`+testcaseInfo+`  steps:
  - type: caller
    code: 200
    trace_dir: %[1]s
    info:
    - "precondition: account · id=7 есть — чтение до проверяемого вызова"
  - type: checker
    out: "1"
    assertions:
    - systemout ShouldNotEqual 0
  - type: caller
    code: 200
    url: /v1/billing/accounts/7
- name: broken
  steps:
  - type: caller
    code: 200
    info:
    - "precondition: account · id=7 есть — чтение до проверяемого вызова"
  - type: checker
    out: "0"
    assertions:
    - systemout ShouldNotEqual 0
  - type: caller
    code: 200
    url: /v1/billing/accounts/7
`, false)
	h := readProtocol(t, dir, "held")
	assert.Contains(t, h, "| account | `id=7` | `caller` | данные есть ✅ |")
	assert.Contains(t, h, "`7` ✅ отправлен")
	b := readProtocol(t, dir, "broken")
	assert.Contains(t, b, "данные есть ❌ не подтверждено", "the check after the read fails the precondition")
}

func TestProtocolGatewayTraceFromHeader(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: gateway
`+testcaseInfo+`  steps:
  - type: gateway
    code: 403
    info:
    - "call: POST /v1/billing/accounts"
  - type: checker
    out: "403"
    traces: %s
    assertions:
    - systemout ShouldEqual 403
`, false)
	p := readProtocol(t, dir, "gateway")
	for _, want := range []string{"## Запрос", "- метод: POST", "- url: http://nginx-gateway/v1/billing/accounts",
		"  - Authorization: Bearer *** (12 знаков)", "  - X-Idempotency-Key: NloBboqV", "\"tenant_id\": \"7\"",
		"## Ответ", "- statuscode: 403", "- trace_id: 0af7651916cd43dd8448eb211c80319c", "  - X-Build: dev-1",
		"  - X-Request-Id: r-1", "  - X-Trace-Id: 0af7651916cd43dd8448eb211c80319c", "\"code\": \"role_required\"",
		"## Трасса", "svc-billing ✅ GetAccount", "✅ пройден"} {
		assert.Contains(t, p, want)
	}
	assert.NotContains(t, p, "secret-token")
	assert.NotContains(t, p, "Server: nginx", "only the trace, request and build headers of the answer")
	assert.NotContains(t, p, "трассы нет")
}

func TestProtocolTraceGateFailsWithoutTree(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: untraced
`+testcaseInfo+`  steps:
  - type: gateway
    code: 403
    trace_dir: %s/missing
`, false)
	p := readProtocol(t, dir, "untraced")
	assert.Contains(t, p, "❌ не пройден")
	assert.Contains(t, p, "trace 0af7651916cd43dd8448eb211c80319c returned by the call has no saved span tree")
	assert.Contains(t, p, "- дерева нет:")
}

func TestProtocolReadBackAfterWrite(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: write
`+testcaseInfo+`  steps:
  - type: caller
    code: 200
    trace_dir: %s
    info:
    - "call: POST /v1/billing/accounts"
  - type: caller
    code: 201
    url: /v1/billing/accounts/7
`, false)
	p := readProtocol(t, dir, "write")
	after := p[strings.Index(p, "## Состояние после вызова"):]
	assert.Contains(t, after, "### шаг 2 «caller»")
	assert.Contains(t, after, "- statuscode: 201")
}
