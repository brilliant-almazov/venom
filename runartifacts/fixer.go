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

// fixer is the service to fix of a testcase: the service and the span of the deepest failed span of its traces and the
// trace it lies in, else the reason it is not established.
type fixer struct {
	service, span, trace, reason string
}

func (f fixer) String() string {
	if f.service == "" {
		return notFound + " (" + f.reason + ")"
	}

	return f.service + " · " + f.span
}

// fixerOf is the service to fix of a testcase that ended: the service and the span of the deepest failed span of its
// traces, the call's first (tempo.Culprit), else «not established (<reason>)».
func fixerOf(view venom.ProtocolView) fixer {
	ids := []string{}
	seen := map[string]bool{}

	for _, id := range append([]string{view.TraceID}, stepTraces(view)...) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return fixer{reason: noTraceID}
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
			return fixer{service: service, span: span, trace: id}
		}

		reason = orFirst(reason, fmt.Sprintf(noFailure, id))
	}

	return fixer{trace: ids[0], reason: reason}
}

func stepTraces(view venom.ProtocolView) []string {
	ids := make([]string, 0, len(view.Steps))

	for _, s := range view.Steps {
		ids = append(ids, s.TraceID)
	}

	return ids
}

func orFirst(first, next string) string {
	if first != "" {
		return first
	}

	return next
}

// fixerOfRow is the service to fix of a row that did not pass; a testcase that did not end has none.
func fixerOfRow(r row) fixer {
	if r.state != stateDone {
		return fixer{reason: notEnded}
	}

	return r.fixer
}

// fixerCell is the service to fix of the row, «—» for a testcase that passed.
func fixerCell(r row) string {
	if passed(r) {
		return noValue
	}

	return cell(fixerOfRow(r).String())
}
