package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOneRunnerOfTheStandUtility: cli, seed, stand_status and env_set start the utility through run only.
func TestOneRunnerOfTheStandUtility(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	starts := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		if n := strings.Count(string(b), "exec.CommandContext("); n > 0 {
			starts[f] = n
		}
	}
	require.Equal(t, map[string]int{"cli.go": 1}, starts)
}
