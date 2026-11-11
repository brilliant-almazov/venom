package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ovh/venom"
)

func TestWritePutsTheTextUnderTheOutputDir(t *testing.T) {
	dir := t.TempDir()
	ctx := context.WithValue(context.Background(), venom.ContextKey("var.venom.outputdir"), dir)
	file, err := Write(ctx, "evidence/B9-10.txt", "request_log.request_log_xmlriver|t\n")
	if err != nil {
		t.Fatal(err)
	}
	if file != filepath.Join(dir, "evidence", "B9-10.txt") {
		t.Errorf("file %s", file)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "request_log.request_log_xmlriver|t\n" {
		t.Errorf("content %q %v", b, err)
	}
	if f, err := Write(ctx, "", "x"); f != "" || err != nil {
		t.Errorf("empty rel wrote %q %v", f, err)
	}
	for _, bad := range []string{"../x.txt", "/tmp/x.txt"} {
		if _, err := Write(ctx, bad, "x"); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
