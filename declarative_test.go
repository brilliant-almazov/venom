package venom

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseSuite(t *testing.T, suite string, lib map[string]string) error {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "suite.yml")
	require.NoError(t, os.WriteFile(file, []byte(suite), 0o644))
	if len(lib) > 0 {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib"), 0o755))
		for name, body := range lib {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", name), []byte(body), 0o644))
		}
	}
	testLoggerOnce.Do(func() { InitTestLogger(t) })
	v := New()
	v.RegisterExecutorBuiltin("caller", callExecutor{})
	if len(lib) > 0 {
		v.LibDir = filepath.Join(dir, "lib")
	}
	return v.Parse(context.Background(), []string{file})
}

func TestDeclarativeRejectsExecAndScriptInSuite(t *testing.T) {
	err := parseSuite(t, `name: suite
testcases:
- name: green
  steps:
  - type: caller
    code: 200
- name: shell
  steps:
  - type: caller
    code: 200
  - type: exec
    script: echo '{{.x}}'
  - type: caller
    script: echo hidden
`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `suite.yml: testcase "shell" step 2: type: exec, script:`)
	assert.Contains(t, err.Error(), `suite.yml: testcase "shell" step 3: script:`)
	assert.NotContains(t, err.Error(), `"green"`)
}

func TestDeclarativeRejectsExecInUserExecutor(t *testing.T) {
	err := parseSuite(t, "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: caller\n    code: 200\n", map[string]string{
		"print.yml": `executor: print
input:
  id: ""
steps:
- type: caller
  code: 200
- type: exec
  command: [python3, render.py]
`,
		"raw.yml": `executor: raw
input: {}
steps:
- type: exec
  script: {{.input.x}}
`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `print.yml: executor "print" step 2: type: exec`)
}

func TestDeclarativeUserExecutorReadByLines(t *testing.T) {
	err := lintUserExecutor("raw.yml", "raw", []byte("executor: raw\nsteps:\n- type: exec\n  script: {{.input.x}}\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `raw.yml:3: executor "raw": - type: exec`)
	assert.Contains(t, err.Error(), `raw.yml:4: executor "raw": script: {{.input.x}}`)
}

func TestDeclarativeAcceptsDeclaredSteps(t *testing.T) {
	assert.NoError(t, parseSuite(t, "name: suite\ntestcases:\n- name: a\n  steps:\n  - type: caller\n    code: 200\n    info:\n    - \"expect: the script of the account is read\"\n", nil))
}
