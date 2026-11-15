// Package writefile writes a file with no shell, and exposes that as the `writefile` executor: a ready mark, a file a
// later step reads.
package writefile

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// Name of the executor.
const Name = "writefile"

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor writes Content to Path, relative to the testsuite folder unless absolute; missing folders are made and
// an existing file is replaced.
type Executor struct {
	Path    string `json:"path" yaml:"path" mapstructure:"path"`
	Content string `json:"content" yaml:"content" mapstructure:"content"`
}

// Result of the executor: the absolute path written and its size in bytes.
type Result struct {
	Path string `json:"path" yaml:"path"`
	Size int    `json:"size" yaml:"size"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// Run writes the file.
func (Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	if e.Path == "" {
		return nil, errors.New("writefile: path is required")
	}
	path := e.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(venom.StringVarFromCtx(ctx, "venom.testsuite.workdir"), path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, []byte(e.Content), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return Result{Path: path, Size: len(e.Content)}, nil
}
