package runartifacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// writeStub writes the protocol of a testcase that did not end — under way, interrupted, never started: its links, its
// steps as the scenario writes them and its verdict with the reason.
func (w *Writer) writeStub(ctx context.Context, r *row) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nFile: `%s`\n\n", r.testcase, r.suiteFile)
	b.WriteString(w.links(r))
	b.WriteString("\n" + headSteps + "\n\n" + transitionsHeader)

	for _, s := range r.view.Steps {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n", s.Number, one(s.Name), one(s.Request), s.Mark, one(s.Reason),
			scenarioLink(dirOf(r.protocol), *r, s.Line))
	}

	b.WriteString("\n" + headVerdict + "\n\n" + venom.MarkNotRun + " " + r.view.Reason + "\n")

	if err := writeFile(r.protocol, []byte(venom.HideSensitive(ctx, b.String()))); err != nil {
		venom.Error(ctx, "unable to write protocol %s: %v", r.protocol, err)
	}
}

// The heads and the table of a protocol written by the run folder.
const (
	headSteps         = "## Steps"
	headVerdict       = "## Verdict"
	transitionsHeader = "| step | name | request | ok / not-ok | reason | scenario |\n| -- | -- | -- | -- | -- | -- |\n"
)
