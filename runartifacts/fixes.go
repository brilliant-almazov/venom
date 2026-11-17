package runartifacts

import (
	"fmt"
	"path/filepath"

	"github.com/gosimple/slug"

	"github.com/ovh/venom"
)

// The fixes of a run: fixes/INDEX.md, a folder per service to fix with its INDEX.md and a file per fix; the testcases
// whose service to fix is not established lie in fixes/not-established with the reason.
const (
	FixesDir          = "fixes"
	NotEstablishedDir = "not-established"
)

// The texts of a fix file and of a testcase that did not pass with no fix file.
const (
	noNorm     = "the scenario names no norm"
	noExpected = "the scenario states no expected behaviour"
	noFixFile  = "testcase %s did not pass and has no fix file"
	normKey    = "norm: "
)

// fix is one fix: the testcases that did not pass with one cause — the same service and span to fix and the same reason
// of their first failed step — and its file fixes/<service>/<NN>-<testcase>.md.
type fix struct {
	fixer  fixer
	reason string
	rows   []row
	file   string
}

// fixService is the service to fix with its folder and its fixes in testcase order.
type fixService struct {
	name, dir string
	fixes     []*fix
}

// planFixes groups every testcase that did not pass into fixes by cause, the fixes into services by the service to fix,
// in testcase order; a testcase whose service is not established lies under not-established.
func (l *layout) planFixes() {
	l.fileOf = map[string]string{}
	byCause := map[string]*fix{}
	at := map[string]int{}

	for _, r := range l.rows {
		if passed(r) {
			continue
		}

		f := fixerOfRow(r)
		reason := stepReason(r)
		key := f.service + "\x00" + f.span + "\x00" + f.reason + "\x00" + reason

		if fx, ok := byCause[key]; ok {
			fx.rows = append(fx.rows, r)
			l.fileOf[rowKey(r)] = fx.file
			continue
		}

		name, dir := f.service, slug.Make(f.service)
		if f.service == "" {
			name, dir = notFound, NotEstablishedDir
		}

		i, ok := at[dir]
		if !ok {
			i = len(l.services)
			at[dir] = i
			l.services = append(l.services, fixService{name: name, dir: filepath.Join(l.dir, FixesDir, dir)})
		}

		s := &l.services[i]
		fx := &fix{fixer: f, reason: reason, rows: []row{r},
			file: filepath.Join(s.dir, fmt.Sprintf("%02d-%s.md", len(s.fixes)+1, venom.ProtocolFileName(r.testcase)))}
		s.fixes = append(s.fixes, fx)
		byCause[key] = fx
		l.fileOf[rowKey(r)] = fx.file
	}
}

func rowKey(r row) string {
	return fmt.Sprintf("%s\x00%d", r.suiteFile, r.index)
}

// stepReason is the reason of the first failed step of the testcase, else the reason of the testcase.
func stepReason(r row) string {
	if s, ok := failedStep(r); ok && s.Reason != "" {
		return one(s.Reason)
	}

	return one(r.view.Reason)
}
