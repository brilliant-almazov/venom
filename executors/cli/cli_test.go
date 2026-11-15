package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// fakeCli prints its argv as a JSON array and exits with the code in FAKE_CLI_CODE.
const fakeCli = `#!/bin/sh
printf '['
sep=''
for a in "$@"; do printf '%s"%s"' "$sep" "$a"; sep=','; done
printf ']\n'
echo warn >&2
exit "${FAKE_CLI_CODE:-0}"
`

func writeFake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cli")
	require.NoError(t, os.WriteFile(bin, []byte(fakeCli), 0o755))
	return bin
}

func TestCliArgvFromFields(t *testing.T) {
	out, err := New().Run(context.Background(), venom.TestStep{"bin": writeFake(t), "action": "service on", "args": []string{"svc-billing-server", "--apply"}, "env": "stand-1"})
	require.NoError(t, err)
	r := out.(Result)
	assert.Equal(t, "0", r.Code)
	assert.Equal(t, `["service","on","svc-billing-server","--apply","--env","stand-1"]`, r.Systemout)
	assert.Equal(t, []interface{}{"service", "on", "svc-billing-server", "--apply", "--env", "stand-1"}, r.SystemoutJSON)
	assert.Equal(t, "warn", r.Systemerr)
}

func TestCliExitCode(t *testing.T) {
	t.Setenv("FAKE_CLI_CODE", "3")
	out, err := New().Run(context.Background(), venom.TestStep{"bin": writeFake(t), "action": "health", "env": "stand-1"})
	require.NoError(t, err)
	assert.Equal(t, "3", out.(Result).Code)
}

func TestCliOutputFile(t *testing.T) {
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.venom.outputdir"), dir)
	out, err := New().Run(ctx, venom.TestStep{"bin": writeFake(t), "action": "health", "env": "s", "output_file": "health/s.txt"})
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(dir, "health", "s.txt"))
	require.NoError(t, err)
	assert.Equal(t, `["health","--env","s"]warn`, string(body))
	assert.Equal(t, filepath.Join(dir, "health", "s.txt"), out.(Result).OutputFile)
}

func TestCliNeedsActionAndEnv(t *testing.T) {
	_, err := New().Run(context.Background(), venom.TestStep{"bin": "x", "env": "s"})
	assert.EqualError(t, err, "cli: action is empty")
	_, err = New().Run(context.Background(), venom.TestStep{"bin": "x", "action": "health"})
	assert.EqualError(t, err, "cli: env is empty")
}

func TestSeedArgvWithBlock(t *testing.T) {
	out, err := NewSeed().Run(context.Background(), venom.TestStep{"bin": writeFake(t), "env": "stand-1", "block": "B02", "action": "ignored"})
	require.NoError(t, err)
	assert.Equal(t, `["seed","--block","B02","--env","stand-1"]`, out.(Result).Systemout)
	_, err = NewSeed().Run(context.Background(), venom.TestStep{"bin": "x", "env": "s"})
	assert.EqualError(t, err, "seed: block is empty")
}

func TestStandStatusArgv(t *testing.T) {
	out, err := NewStandStatus().Run(context.Background(), venom.TestStep{"bin": writeFake(t), "env": "stand-1"})
	require.NoError(t, err)
	assert.Equal(t, `["status","--env","stand-1"]`, out.(Result).Systemout)
}

// fakeHealth prints health rows, the red ones with a mark and a duration.
const fakeHealth = `#!/bin/sh
echo "  ✓ postgres 0.10 s"
echo "  ✗ svc-billing-server 1.20 s"
echo "  ✗ rabbitmq 0.31 s" >&2
`

func TestCliHealthRed(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "cli")
	require.NoError(t, os.WriteFile(bin, []byte(fakeHealth), 0o755))
	out, err := New().Run(context.Background(), venom.TestStep{"bin": bin, "action": "health", "env": "s", "baseline": "✗ rabbitmq 0.02 s\n  ✓ postgres 0.1 s"})
	require.NoError(t, err)
	r := out.(Result)
	assert.Equal(t, "✗ rabbitmq\n✗ svc-billing-server", r.Red, "the red rows of systemout and systemerr, normalised and sorted")
	assert.Equal(t, "✗ svc-billing-server", r.RedNew, "only the rows not red in the baseline")
}

func TestCliRedOnlyForHealth(t *testing.T) {
	out, err := New().Run(context.Background(), venom.TestStep{"bin": writeFake(t), "action": "service status", "env": "s"})
	require.NoError(t, err)
	assert.Empty(t, out.(Result).Red)
}
