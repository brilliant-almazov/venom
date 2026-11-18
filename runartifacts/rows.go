package runartifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ovh/venom"
)

// rowOf is the row of testcase index of ts, made on first sight; its protocol is named not-run until the testcase ends
// with an outcome of its own, so a testcase interrupted, not run or pending carries the outcome not-run, never failed;
// it lies in the folder of its testsuite named by the outcome of the testsuite (suiteDir).
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
		index: index, trees: map[string]string{}, state: statePending, outcome: venom.ProtocolOutcomeNotRun}
	r.protocol = filepath.Join(w.folderOf(r.suiteShort), venom.ProtocolOutcomeName(name, venom.ProtocolOutcomeNotRun)+".md")
	w.rows[key] = r

	return r
}

// suiteDir is the folder protocols/<row id>-<outcome>-<rest> of the testsuite short: ok when every testcase of it
// ended ok, partial when some testcase is ok or partial, failed when some testcase failed, not-run otherwise — a
// testcase that did not end is not-run, never failed (the outcome belongs in the folder name too).
func (w *Writer) suiteDir(short string) string {
	all, some, fail, seen := true, false, false, false

	for _, r := range w.rows {
		if r.suiteShort != short {
			continue
		}

		seen = true
		all = all && r.outcome == venom.ProtocolOutcomeOK
		some = some || r.outcome == venom.ProtocolOutcomeOK || r.outcome == venom.ProtocolOutcomePartial
		fail = fail || r.outcome == venom.ProtocolOutcomeFailed
	}

	o := venom.ProtocolOutcomeNotRun

	switch {
	case seen && all:
		o = venom.ProtocolOutcomeOK
	case some:
		o = venom.ProtocolOutcomePartial
	case fail:
		o = venom.ProtocolOutcomeFailed
	}

	return filepath.Join(w.dir, ProtocolsDir, venom.ProtocolOutcomeName(short, o))
}

// folderOf is the folder the testsuite short lies in now: that of a row of it already seen, else the folder of a
// testsuite whose testcases did not end, not-run.
func (w *Writer) folderOf(short string) string {
	for _, r := range w.rows {
		if r.suiteShort == short {
			return filepath.Dir(r.protocol)
		}
	}

	return filepath.Join(w.dir, ProtocolsDir, venom.ProtocolOutcomeName(short, venom.ProtocolOutcomeNotRun))
}

// moveSuite moves the folder of the testsuite of r to the name of its outcome, every protocol of the testsuite with it;
// true when it moved, so the whole run folder is redrawn with the new links.
func (w *Writer) moveSuite(ctx context.Context, r *row) bool {
	from, to := filepath.Dir(r.protocol), w.suiteDir(r.suiteShort)
	if from == to {
		return false
	}

	if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
		venom.Error(ctx, "unable to move protocols %s to %s: %v", from, to, err)
	}

	for _, o := range w.rows {
		if o.suiteShort == r.suiteShort {
			o.protocol = filepath.Join(to, filepath.Base(o.protocol))
		}
	}

	return true
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
