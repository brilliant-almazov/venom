// Package evidence writes the raw output of a step to a file under the venom output dir, so a read-back step that
// makes no traced call leaves its output as the record of the verdict.
package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ovh/venom"
)

// Write puts text to <venom.outputdir>/<rel> (the testsuite workdir when the run has no output dir) and returns the
// absolute path; an empty rel writes nothing. rel is relative and stays inside the output dir.
func Write(ctx context.Context, rel, text string) (string, error) {
	if rel == "" {
		return "", nil
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(filepath.Clean(rel), "..") {
		return "", fmt.Errorf("output_file %q: a relative path inside the output dir", rel)
	}
	dir := venom.StringVarFromCtx(ctx, "venom.outputdir")
	if dir == "" {
		dir = venom.StringVarFromCtx(ctx, "venom.testsuite.workdir")
	}
	file := filepath.Join(dir, filepath.Clean(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return file, nil
	}
	return abs, nil
}
