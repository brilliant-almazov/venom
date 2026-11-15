package writefile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ovh/venom"
	"github.com/ovh/venom/internal/golden"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.venom.testsuite.workdir"), dir)
	out, err := Executor{}.Run(ctx, venom.TestStep{"path": "results/stand/S0-R1.ready", "content": "valkey on\nrecords 3\n"})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	body, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != filepath.Join(dir, "results/stand/S0-R1.ready") || r.Size != len(body) {
		t.Fatalf("path %s size %d", r.Path, r.Size)
	}
	golden.Text(t, "ready_mark", string(body))

	if _, err := (Executor{}).Run(ctx, venom.TestStep{"content": "x"}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("no path: %v", err)
	}
}
