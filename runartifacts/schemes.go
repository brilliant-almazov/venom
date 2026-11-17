package runartifacts

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ovh/venom"
)

// findSchemes is the folder schemes of the testsuite set: the first one met going up from the folder of the testsuite
// file, empty when there is none.
func findSchemes(suiteFile string) string {
	abs, err := filepath.Abs(suiteFile)
	if err != nil {
		return ""
	}

	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		if st, err := os.Stat(filepath.Join(dir, SchemesDir)); err == nil && st.IsDir() {
			return filepath.Join(dir, SchemesDir)
		}

		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

// fillSchemes copies every scheme folder of the run to <run>/schemes with each anchor replaced by the run (anchors.go);
// the rest of the text is kept as it is.
func (w *Writer) fillSchemes(ctx context.Context) error {
	seen := map[string]bool{}

	for _, r := range w.rows {
		if r.schemes == "" || seen[r.schemes] {
			continue
		}

		seen[r.schemes] = true

		if err := w.fillFolder(r.schemes); err != nil {
			venom.Error(ctx, "unable to fill the schemes of %s: %v", r.schemes, err)
			return err
		}
	}

	return nil
}

func (w *Writer) fillFolder(src string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		to := filepath.Join(w.dir, SchemesDir, rel)
		filled := fill{rows: w.rows, from: filepath.Dir(to)}.text(string(body))

		return writeFile(to, []byte(filled))
	})
}
