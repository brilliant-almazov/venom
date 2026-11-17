package runartifacts

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// removeEmptyDirs removes every empty folder under dir, deepest first, so a folder stays only with a file in it (run
// 17-4 left an empty raw/identifier-link/).
func removeEmptyDirs(dir string) {
	var dirs []string

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != dir {
			dirs = append(dirs, path)
		}

		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})

	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
}
