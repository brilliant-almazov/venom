package runartifacts

import (
	"fmt"
	"os"
	"strings"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// The service to fix of a testcase that did not pass, as PROBLEMS.md, REPORT.md and its protocol name it explicitly.
const (
	wordFixer  = "fix in"
	notFound   = "not established"
	notEnded   = "the testcase did not end"
	noTraceID  = "the call returned no trace id"
	noSpanTree = "no span tree saved for trace %s"
	noFailure  = "no span failed in trace %s"
)

// fixerOf is the service to fix of a testcase that ended: the service and the span of the deepest failed span of its
// traces, the call's first (tempo.Culprit), else «not established (<reason>)».
func fixerOf(view venom.ProtocolView) string {
	ids := []string{}
	seen := map[string]bool{}

	for _, id := range append([]string{view.TraceID}, stepTraces(view)...) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return unknownFixer(noTraceID)
	}

	reason := ""

	for _, id := range ids {
		f, ok := view.Trees[id]
		if !ok {
			reason = orFirst(reason, fmt.Sprintf(noSpanTree, id))
			continue
		}

		body, err := os.ReadFile(strings.TrimSuffix(f, ".md") + ".json")
		if err != nil {
			reason = orFirst(reason, fmt.Sprintf(noSpanTree, id))
			continue
		}

		if service, span, ok := tempo.Culprit(body); ok {
			return service + " · " + span
		}

		reason = orFirst(reason, fmt.Sprintf(noFailure, id))
	}

	return unknownFixer(reason)
}

func stepTraces(view venom.ProtocolView) []string {
	ids := make([]string, 0, len(view.Steps))

	for _, s := range view.Steps {
		ids = append(ids, s.TraceID)
	}

	return ids
}

func unknownFixer(reason string) string {
	return notFound + " (" + reason + ")"
}

func orFirst(first, next string) string {
	if first != "" {
		return first
	}

	return next
}

// fixerCell is the service to fix of the row, «—» for a testcase that passed.
func fixerCell(r row) string {
	switch {
	case passed(r):
		return noValue

	case r.state != stateDone:
		return unknownFixer(notEnded)

	default:
		return cell(r.fixer)
	}
}

// fixersOf are the services to fix of the rows that did not pass, each once, «—» when every one passed.
func fixersOf(rows []row) string {
	var out []string
	seen := map[string]bool{}

	for _, r := range rows {
		if passed(r) {
			continue
		}

		if f := fixerCell(r); !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}

	if len(out) == 0 {
		return noValue
	}

	return strings.Join(out, "; ")
}
