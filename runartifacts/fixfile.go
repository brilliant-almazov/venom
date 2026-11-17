package runartifacts

import (
	"fmt"
	"path/filepath"
	"strings"
)

// writeFix writes the file of one fix: what to do — expected and got of the failed step, its call, code and body —, why
// — the norm of each testcase with its scenario line —, where — the service, the span of the failure and the trace —,
// and the links of its testcases.
func (l layout) writeFix(s fixService, f *fix) error {
	base := s.dir
	var b strings.Builder
	fmt.Fprintf(&b, "# %s · %s\n\n", strings.TrimSuffix(filepath.Base(f.file), ".md"), s.name)
	fmt.Fprintf(&b, "- %s: %s\n- reason: %s\n- testcases: %d\n", wordFixer, cell(f.fixer.String()), orNone(f.reason), len(f.rows))
	b.WriteString("\n## Where\n\n")

	switch f.fixer.service {
	case "":
		fmt.Fprintf(&b, "- service: %s (%s)\n", notFound, f.fixer.reason)

	default:
		fmt.Fprintf(&b, "- service: %s\n- span of the failure: %s\n", f.fixer.service, f.fixer.span)
	}

	for _, r := range f.rows {
		for _, id := range traceIDs(r) {
			fmt.Fprintf(&b, "- trace id of %s: %s\n", r.testcase, traceRef(base, r, id))
		}

		if len(traceIDs(r)) == 0 {
			fmt.Fprintf(&b, "- trace id of %s: %s (%s)\n", r.testcase, noValue, noTraceID)
		}
	}

	b.WriteString("\n## What to do\n")

	for _, r := range f.rows {
		writeExpectedGot(&b, base, r)
	}

	b.WriteString("\n## Why\n\n")

	for _, r := range f.rows {
		norms := expectedLines(r, true)
		if len(norms) == 0 {
			norms = []string{noNorm}
		}

		for _, n := range norms {
			fmt.Fprintf(&b, "- %s: %s — %s\n", r.testcase, n, scenarioLink(base, r, r.view.Line))
		}
	}

	b.WriteString("\n## Links\n\n")

	for _, r := range f.rows {
		protocol := noValue

		if written(r) {
			protocol = link(protocolLink, base, r.protocol)
		}

		fmt.Fprintf(&b, "- %s: %s · %s\n", r.testcase, protocol, link(wordScheme, base, l.caseScheme(r)))
	}

	fmt.Fprintf(&b, "- run: %s · %s · %s · %s\n", link(ReportFile, base, l.report()), link(ProblemsFile, base, l.problems()),
		link(FixesDir+"/"+IndexFile, base, l.fixesIndex()), link(s.name, base, filepath.Join(s.dir, IndexFile)))

	return writeFile(f.file, []byte(b.String()))
}
