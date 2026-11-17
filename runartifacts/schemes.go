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
	for _, dir := range w.schemeFolders() {
		if err := w.fillFolder(dir); err != nil {
			venom.Error(ctx, "unable to fill the schemes of %s: %v", dir, err)
			return err
		}
	}

	return nil
}

// schemeFolders are the folders of the schemes drawn at intake. First the scheme of the suite in QA,
// <qa>/schemes/<suite> for a run folder <qa>/cases/<suite>/runs/<YYYY-MM>/<DD-n> (qaSchemes). Without it, the schemes
// folders found up from the testsuite files. With none of those, the schemes folder of the case set the run folder
// belongs to, <set>/schemes, when the testsuites live in another repository than the schemes.
func (w *Writer) schemeFolders() []string {
	if qa := qaSchemes(w.dir); qa != "" {
		return []string{qa}
	}

	var dirs []string
	seen := map[string]bool{}

	for _, r := range w.layout().rows {
		if r.schemes == "" || seen[r.schemes] {
			continue
		}

		seen[r.schemes] = true
		dirs = append(dirs, r.schemes)
	}

	if len(dirs) > 0 {
		return dirs
	}

	set := filepath.Join(w.dir, "..", "..", "..", SchemesDir)

	if st, err := os.Stat(set); err == nil && st.IsDir() {
		return []string{set}
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
		filled := fill{rows: w.layout().rows, from: filepath.Dir(to)}.text(string(body))

		return writeFile(to, []byte(filled))
	})
}

// qaSchemes is the scheme of the suite in QA for the run folder dir, <qa>/cases/<suite>/runs/<YYYY-MM>/<DD-n> →
// <qa>/schemes/<suite>; empty when dir is not laid out so or QA has no scheme of the suite.
func qaSchemes(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	set := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	cases := filepath.Dir(set)

	if filepath.Base(filepath.Dir(filepath.Dir(abs))) != runsDir || filepath.Base(cases) != "cases" {
		return ""
	}

	qa := filepath.Join(filepath.Dir(cases), SchemesDir, filepath.Base(set))

	if st, err := os.Stat(qa); err != nil || !st.IsDir() {
		return ""
	}

	return qa
}
