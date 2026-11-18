package runartifacts

import (
	"fmt"
	"os"
	"strings"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// The service to fix of a testcase — the zone of fix responsibility —, as every file of the run folder names it
// explicitly.
const (
	wordFixer  = "fix in"
	headFix    = "Fix responsibility"
	wordBoth   = "both"
	fixPassed  = "nothing to fix: the testcase passed"
	whyCulprit = "the deepest failed span of trace %s"
	whyBoth    = "the deepest failed span of each trace of the testcase"
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

// fixerOf is the service to fix of a testcase that ended: the service and the span of the deepest failed span of each
// of its traces, the call's first (tempo.Culprit) — «both: …» when they lie in more than one service —, else «not
// established (<reason>)».
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
	var found []fixer

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
			found = append(found, fixer{service: service, span: span, trace: id})
			continue
		}

		reason = orFirst(reason, fmt.Sprintf(noFailure, id))
	}

	if len(found) == 0 {
		return fixer{trace: ids[0], reason: reason}
	}

	return bothOf(found)
}

// bothOf is the service to fix of the culprits of the traces of a testcase, the call's first: that service when they
// all lie in one, else «both: <service> + <service>» with the span of each.
func bothOf(found []fixer) fixer {
	first := found[0]
	services, spans := []string{first.service}, []string{first.span}
	seen := map[string]bool{first.service: true}

	for _, f := range found[1:] {
		if !seen[f.service] {
			seen[f.service] = true
			services = append(services, f.service)
			spans = append(spans, f.span)
		}
	}

	if len(services) == 1 {
		return first
	}

	return fixer{service: wordBoth + ": " + strings.Join(services, " + "), span: strings.Join(spans, " + "), trace: first.trace}
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

// fixDetail is the zone of fix responsibility of the row in full, for every testcase: «<service> · <span> — the deepest
// failed span of trace <id>», «not established (<reason>)», «— (nothing to fix: the testcase passed)».
func fixDetail(r row) string {
	if passed(r) {
		return noValue + " (" + fixPassed + ")"
	}

	f := fixerOfRow(r)

	switch {
	case f.service == "":
		return f.String()

	case strings.HasPrefix(f.service, wordBoth+": "):
		return f.String() + " — " + whyBoth

	default:
		return f.String() + " — " + fmt.Sprintf(whyCulprit, f.trace)
	}
}

// fixCell is the zone of fix responsibility of the row in full, linked to its fix file when it did not pass.
func (l layout) fixCell(from string, r row) string {
	file, ok := l.fileOf[rowKey(r)]
	if !ok {
		return cell(fixDetail(r))
	}

	return link(cell(fixDetail(r)), from, file)
}
