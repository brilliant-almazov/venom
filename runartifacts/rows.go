package runartifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gosimple/slug"

	"github.com/ovh/venom"
)

// rowOf is the row of testcase index of ts, made on first sight.
func (w *Writer) rowOf(ts *venom.TestSuite, index int) *row {
	key := fmt.Sprintf("%s\x00%d", ts.Filepath, index)
	if r, ok := w.rows[key]; ok {
		return r
	}

	name := ""
	if index < len(ts.TestCases) {
		name = venom.CaseName(&ts.TestCases[index])
	}

	abs, err := filepath.Abs(ts.Filepath)
	if err != nil {
		abs = ts.Filepath
	}

	r := &row{suite: ts.Name, suiteShort: ts.ShortName, suiteFile: ts.Filepath, absFile: abs, testcase: name,
		index: index, trees: map[string]string{}, state: statePending,
		protocol: filepath.Join(w.dir, ProtocolsDir, slug.Make(ts.ShortName), venom.ProtocolFileName(name)+".md")}
	w.rows[key] = r

	return r
}

func caseIndex(ts *venom.TestSuite, tc *venom.TestCase) int {
	for i := range ts.TestCases {
		if &ts.TestCases[i] == tc {
			return i
		}
	}

	return len(ts.TestCases)
}

// writeFile writes body to file, its folder made with it.
func writeFile(file string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}

	return os.WriteFile(file, body, 0o644)
}

// copyHidden copies a span tree with the secrets of the run hidden, as the protocol that prints it.
func copyHidden(ctx context.Context, from, to string) error {
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}

	return writeFile(to, []byte(venom.HideSensitive(ctx, string(body))))
}
