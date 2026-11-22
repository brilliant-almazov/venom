package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

func TestEnvSetArgv(t *testing.T) {
	set, reset, err := EnvSet{Service: "svc-crawler", Values: map[string]string{"B": "2", "A": "1"}, Env: "stand-1"}.argv()
	require.NoError(t, err)
	assert.Equal(t, []string{"var", "set", "svc-crawler", "A=1", "B=2", "--restart", "--env", "stand-1"}, set)
	assert.Equal(t, []string{"var", "reset", "svc-crawler", "--restart", "--env", "stand-1"}, reset)
}

func TestEnvSetRefusesAnIncompleteStep(t *testing.T) {
	for _, e := range []EnvSet{
		{Values: map[string]string{"A": "1"}, Env: "stand-1"},
		{Service: "svc-crawler", Env: "stand-1"},
		{Service: "svc-crawler", Values: map[string]string{"A": "1"}},
	} {
		_, _, err := e.argv()
		require.Error(t, err)
	}
}

func TestEnvSetEnvFromTheStandVar(t *testing.T) {
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.stand"), "stand-7")
	assert.Equal(t, "stand-7", EnvSet{}.envOf(ctx))
	assert.Equal(t, "stand-1", EnvSet{Env: "stand-1"}.envOf(ctx))
}

func TestEnvSetOutOfATestCaseIsRefused(t *testing.T) {
	step := venom.TestStep{"bin": writeFake(t), "service": "svc-crawler", "values": map[string]interface{}{"A": 1}, "env": "stand-1"}
	_, err := NewEnvSet().Run(context.Background(), step)
	require.ErrorIs(t, err, venom.ErrNoTestCase)
}
