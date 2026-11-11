package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ovh/venom"
)

func TestOutputFileKeepsTheRawOutput(t *testing.T) {
	venom.InitTestLogger(t)
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.venom.outputdir"), dir)
	out, err := Executor{}.Run(ctx, venom.TestStep{"script": "echo 'svc-crawler-worker PARTITIONS_RETENTION=0'", "output_file": "evidence/B11-116.txt"})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	want := filepath.Join(dir, "evidence", "B11-116.txt")
	if r.OutputFile != want {
		t.Errorf("output_file %q want %q", r.OutputFile, want)
	}
	b, _ := os.ReadFile(want)
	if string(b) != "svc-crawler-worker PARTITIONS_RETENTION=0" {
		t.Errorf("file %q", b)
	}
}
