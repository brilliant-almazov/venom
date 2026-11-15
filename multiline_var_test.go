package venom

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordExecutor keeps the value field of each step it runs.
type recordExecutor struct {
	mu   *sync.Mutex
	seen *[]string
}

func (r recordExecutor) Run(_ context.Context, step TestStep) (interface{}, error) {
	v, _ := step.StringValue("value")
	r.mu.Lock()
	*r.seen = append(*r.seen, v)
	r.mu.Unlock()
	return map[string]interface{}{"value": v}, nil
}

func TestMultiLineVarKeepsItsLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "suite.yml")
	require.NoError(t, os.WriteFile(file, []byte(`name: suite
testcases:
- name: lines
  steps:
  - type: recorder
    value: "  ✗ rabbitmq 0.31 s\n  ✗ svc-billing-server 1.20 s"
    vars:
      before:
        from: value
  - type: recorder
    value: '{{.before}}'
  - type: recorder
    value: 'tab{{"\t"}}and "quote"'
`), 0o644))
	testLoggerOnce.Do(func() { InitTestLogger(t) })
	var seen []string
	v := New()
	v.RegisterExecutorBuiltin("recorder", recordExecutor{mu: &sync.Mutex{}, seen: &seen})
	v.OutputDir = dir
	ctx := context.Background()
	require.NoError(t, v.Parse(ctx, []string{file}))
	require.NoError(t, v.Process(ctx, []string{file}))
	require.Len(t, seen, 3)
	assert.Equal(t, "  ✗ rabbitmq 0.31 s\n  ✗ svc-billing-server 1.20 s", seen[1], "a two-line var reaches the step field with its newline")
	assert.Equal(t, "tab\tand \"quote\"", seen[2])
}
