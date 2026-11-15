// Package golden compares a test result with testdata/<name>.golden; `go test -update` rewrites the file.
package golden

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// JSON compares v as indented JSON with testdata/<name>.golden.
func JSON(t *testing.T, name string, v interface{}) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	Text(t, name, string(b)+"\n")
}

// Text compares got with testdata/<name>.golden.
func Text(t *testing.T, name, got string) {
	t.Helper()
	file := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: %v (run go test -update)", file, err)
	}
	if string(want) != got {
		t.Fatalf("%s differs\ngot\n%s\nwant\n%s", file, got, want)
	}
}
