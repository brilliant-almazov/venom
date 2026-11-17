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
	TraceID    string            `json:"trace_id"`
	Code       *int              `json:"code,omitempty"`
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
	require.NoError(t, os.WriteFile(filepath.Join(traces, "0af7651916cd43dd8448eb211c80319c.md"), []byte("svc-a ✅ Get"), 0o644))
	file := filepath.Join(dir, "suite.yml")
	require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf(suite, traces)), 0o644))

	testLoggerOnce.Do(func() { InitTestLogger(t) })
	v := New()
	v.RegisterExecutorBuiltin("caller", callExecutor{})
	v.RegisterExecutorBuiltin("checker", checkExecutor{})
	v.RegisterExecutorBuiltin("gateway", gatewayExecutor{})
	v.RegisterExecutorBuiltin("wait", waitExecutor{})
	v.RegisterExecutorBuiltin("stand", standExecutor{})
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
	base := filepath.Join(dir, "suite", protocolFileName(testcase))
	files, err := filepath.Glob(base + protocolOutcomeSep + "*.md")
	require.NoError(t, err)
	if _, err := os.Stat(base + ".md"); err == nil {
		files = append(files, base+".md")
	}
	require.Len(t, files, 1, "protocol of %s", testcase)
	btes, err := os.ReadFile(files[0])
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
    method: GET
    url: /v1/billing/accounts/1
    trace_dir: %s
    info:
    - "call: GET /v1/billing/accounts/1"
    assertions:
    - result.statuscode ShouldEqual 200
`, false)
	p := readProtocol(t, dir, "green")
	for _, want := range []string{protocolHeadRight, "- expect: the account is read", "- right: statuscode 200", "- norm: cgw-billing: docs/api.md:12",
		protocolHeadWrong, "- wrong: any other statuscode", protocolHeadRequest, protocolHeadAnswer, protocolHeadAfter, "- call: `caller GET /v1/billing/accounts/1`", "- statuscode: 200",
		protocolHeadTrace, "svc-a ✅ Get", protocolHeadVerdict, protocolPassed} {
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
	assert.Contains(t, p, protocolFailed)
	assert.Contains(t, p, "result.statuscode MustEqual 200")
	assert.Contains(t, p, "- statuscode: 500")
	assert.Contains(t, p, "svc-a ✅ Get")
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
	assert.Contains(t, first, protocolFailed)
	assert.Contains(t, first, "result.statuscode ShouldEqual 200")
	second := readProtocol(t, dir, "second")
	assert.Contains(t, second, protocolSkipped)
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
	assert.Contains(t, p, protocolFailed)
	assert.Contains(t, p, "result.statuscode ShouldEqual 200")
	assert.Contains(t, p, protocolHeadAnswer+"\n\n- statuscode: 422", "the failed call is the fact even though its vars were skipped")
	assert.Contains(t, p, "svc-a ✅ Get")
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
	assert.Contains(t, p, protocolFailed)
	assert.Contains(t, p, "systemout ShouldEqual 200")
	assert.Contains(t, p, "- statuscode: 404", "the call is the fact, not the check step after it")
	assert.Contains(t, p, "svc-a ✅ Get")
}

const presenceSuite = `name: suite
testcases:
- name: present
` + testcaseInfo + `  steps:
  - type: caller
    code: 200
    method: GET
    url: /v1/billing/accounts/7
    trace_dir: %[1]s
    info:
    - "call: GET /v1/billing/accounts/7"
    - "precondition: account · id=7 ` + protocolWordPresent + `"
    - "expect: ` + protocolDataPresent + `: account · id=7, the account is read"
    assertions:
    - result.statuscode MustEqual 200
  - type: caller
    code: 200
    url: /v1/billing/accounts/7/entries
    info:
    - "expect: ` + protocolDataPresent + `: account · id=7, the entries are read"
- name: absent
  steps:
  - type: caller
    code: 404
    method: GET
    url: /v1/billing/accounts/9223372036854775807
    info:
    - "call: GET /v1/billing/accounts/9223372036854775807"
    - "precondition: account · id=9223372036854775807 ` + protocolWordAbsent + `"
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
    - "precondition: account · id=7 ` + protocolWordPresent + `"
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
	assert.Contains(t, p, protocolHeadPresence+"\n\n"+protocolPresenceHeader)
	assert.Contains(t, p, "| account | `id=7` | `caller GET /v1/billing/accounts/7` | "+protocolDataPresent+" ✅ | `0af7651916cd43dd8448eb211c80319c` | `7` "+protocolKeySent)
	assert.Less(t, strings.Index(p, protocolHeadPresence), strings.Index(p, protocolHeadRequest), "the table goes before the main call")
	before := p[strings.Index(p, protocolHeadBefore):strings.Index(p, protocolHeadPresence)]
	assert.Contains(t, before, fmt.Sprintf(protocolAfterStep, 1, "caller", ""), "the precondition read with its request and answer")
	assert.Contains(t, before, "- statuscode: 200")
	assert.Equal(t, 1, strings.Count(p, "| account |"), "the expect line naming present data makes no row")

	a := readProtocol(t, dir, "absent")
	assert.Contains(t, a, "| account | `id=9223372036854775807` | `caller GET /v1/billing/accounts/9223372036854775807` | "+protocolDataAbsent+" ✅ |")
	assert.Contains(t, a, "`9223372036854775807` "+protocolKeyNotSent, "the tested call looked for another id")

	f := readProtocol(t, dir, "failed")
	assert.Contains(t, f, protocolDataPresent+" "+protocolNotConfirmed)
	assert.Contains(t, f, protocolFailed)
	assert.Contains(t, f, "result.statuscode MustEqual 200", "the testcase fails at the precondition")
	assert.Contains(t, f, fmt.Sprintf(protocolStepLine, 1, "caller"), "the fact is the failed precondition read")

	n := readProtocol(t, dir, "none")
	assert.Contains(t, n, protocolHeadPresence+"\n\n"+protocolNoPrecondition+"\n")
	assert.NotContains(t, n, protocolPresenceHeader)
}

func TestProtocolFileNameKeepsRowID(t *testing.T) {
	assert.Equal(t, "GW-B0005-DELETE-v1-billing-accounts-{account_id}-without-key", protocolFileName("GW-B0005 DELETE /v1/billing/accounts/{account_id} without key"))
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
    - "precondition: account · id=7 `+protocolWordPresent+`"
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
    - "precondition: account · id=7 `+protocolWordPresent+`"
  - type: checker
    out: "0"
    assertions:
    - systemout ShouldNotEqual 0
  - type: caller
    code: 200
    url: /v1/billing/accounts/7
`, false)
	h := readProtocol(t, dir, "held")
	assert.Contains(t, h, "| account | `id=7` | `caller` | "+protocolDataPresent+" ✅ |")
	assert.Contains(t, h, "`7` "+protocolKeySent)
	b := readProtocol(t, dir, "broken")
	assert.Contains(t, b, protocolDataPresent+" "+protocolNotConfirmed, "the check after the read fails the precondition")
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
	for _, want := range []string{protocolHeadRequest, protocolMethodLine + "POST", "- url: http://nginx-gateway/v1/billing/accounts",
		"  - Authorization: Bearer " + SecretMark, "  - X-Idempotency-Key: NloBboqV", "\"tenant_id\": \"7\"",
		protocolHeadAnswer, "- statuscode: 403", "- trace_id: 0af7651916cd43dd8448eb211c80319c", "  - X-Build: dev-1",
		"  - X-Request-Id: r-1", "  - X-Trace-Id: 0af7651916cd43dd8448eb211c80319c", "\"code\": \"role_required\"",
		protocolHeadTrace, "svc-a ✅ Get", protocolPassed} {
		assert.Contains(t, p, want)
	}
	assert.NotContains(t, p, "secret-token")
	assert.NotContains(t, p, "Server: nginx", "only the trace, request and build headers of the answer")
	assert.NotContains(t, p, protocolNoTrace)
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
	assert.Contains(t, p, protocolFailed)
	assert.Contains(t, p, "trace 0af7651916cd43dd8448eb211c80319c returned by the call has no saved span tree")
	assert.Contains(t, p, protocolNoTree)
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
	after := p[strings.Index(p, protocolHeadAfter):]
	assert.Contains(t, after, fmt.Sprintf(protocolAfterStep, 2, "caller", ""))
	assert.Contains(t, after, "- statuscode: 201")
}

func TestProtocolCallIsDeclarative(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: shell
`+testcaseInfo+`  steps:
  - type: caller
    code: 200
    trace_dir: %[1]s
    info:
    - "call: curl -s http://nginx-gateway/v1/billing/accounts"
- name: stand
`+testcaseInfo+`  steps:
  - type: caller
    code: 200
    action: service on
    args: [svc-billing-server, --apply]
    env: stand-1
    info:
    - "call: bin/cli service on svc-billing-server --apply --env stand-1 | grep ok"
`, false)
	p := readProtocol(t, dir, "shell")
	assert.NotContains(t, p, "curl", "the call line is rendered from the step, not from the declared text")
	assert.Contains(t, p, "- call: `caller`")
	s := readProtocol(t, dir, "stand")
	assert.Contains(t, s, "- call: `caller action=service on args=svc-billing-server --apply env=stand-1`")
	assert.Contains(t, s, "- action: service on\n- args: svc-billing-server --apply\n- env: stand-1\n")
	assert.NotContains(t, s, "grep")
}

// waitExecutor stands for the wait executor of executors/wait, which the root package cannot import.
type waitExecutor struct{}

func (waitExecutor) Run(_ context.Context, _ TestStep) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func TestProtocolWaitLine(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: waited
`+testcaseInfo+`  steps:
  - type: wait
    seconds: "0.01"
    reason: the absence window before the read
  - type: wait
    seconds: 2
  - type: caller
    code: 200
    trace_dir: %s
`, false)
	p := readProtocol(t, dir, "waited")
	req := p[strings.Index(p, protocolHeadRequest):strings.Index(p, protocolHeadAnswer)]
	assert.Contains(t, req, fmt.Sprintf(protocolWaitLine, 1, "0.01", "the absence window before the read"))
	assert.Contains(t, req, fmt.Sprintf(protocolWaitLine, 2, "2", protocolNoReason))
}

func TestProtocolExpectedFromInfoKeysOnly(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: declared
  info:
  - "happens: the account is read by id"
  - "right: statuscode 200"
  - "wrong: any other statuscode"
  - "norm: cgw-billing: docs/api.md:12"
  steps:
  - type: caller
    code: 200
    method: GET
    url: /v1/billing/accounts/1
    trace_dir: %s
    info:
    - "call: curl -s http://nginx-gateway/v1/billing/accounts/1"
    - "expect: the account is read"
`, false)
	p := readProtocol(t, dir, "declared")
	right := p[strings.Index(p, protocolHeadRight):strings.Index(p, protocolHeadWrong)]
	assert.Equal(t, protocolHeadRight+"\n\n- happens: the account is read by id\n- expect: the account is read\n- right: statuscode 200\n"+
		"- norm: cgw-billing: docs/api.md:12\n\n", right)
	assert.NotContains(t, p, "curl", "the call: text is printed nowhere")
}

type standExecutor struct{}

func (standExecutor) Run(_ context.Context, step TestStep) (interface{}, error) {
	code, _ := step.IntValue("code")
	return Result{Code: &code}, nil
}

func TestProtocolFileNameCarriesOutcome(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: green
  steps:
  - type: caller
    code: 200
    trace_dir: %[1]s
    assertions:
    - result.statuscode ShouldEqual 200
- name: red
  steps:
  - type: caller
    code: 500
    trace_dir: %[1]s
    assertions:
    - result.statuscode ShouldEqual 200
- name: half
  steps:
  - type: caller
    code: 200
    trace_dir: %[1]s
    assertions:
    - result.statuscode ShouldEqual 200
  - type: caller
    code: 500
    assertions:
    - result.statuscode ShouldEqual 200
`, false)
	for name, outcome := range map[string]string{"green": protocolOutcomeOK, "red": protocolOutcomeFailed, "half": protocolOutcomePartial} {
		_, err := os.Stat(filepath.Join(dir, "suite", name+protocolOutcomeSep+outcome+".md"))
		assert.NoError(t, err, name)
	}
}

func TestProtocolTraceOfEveryStepWhenTheCallHasNone(t *testing.T) {
	dir := runProtocolSuite(t, `name: suite
testcases:
- name: teardown
  steps:
  - type: stand
    code: 1
    assertions:
    - result.code ShouldEqual 0
  - type: caller
    code: 500
    trace_dir: %s
    assertions:
    - result.statuscode ShouldEqual 500
`, false)
	p := readProtocol(t, dir, "teardown")
	assert.Contains(t, p, protocolHeadTrace+"\n\n- "+protocolNoTrace+": the call returned no trace_id")
	assert.Contains(t, p, fmt.Sprintf(protocolStepLine, 2, "caller")+"\n- trace_id: 0af7651916cd43dd8448eb211c80319c")
	assert.Contains(t, p, "svc-a ✅ Get")
}
