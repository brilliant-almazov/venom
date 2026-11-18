package runartifacts

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ovh/venom"
)

// anchor is a mark of a scheme template: ⟦<case>⟧, ⟦<case>:<hop>⟧, ⟦<case>:<hop>:reason⟧, ⟦*:<hop>⟧,
// ⟦<case>:protocol⟧, ⟦<case>:trace⟧, ⟦<case>:previous⟧, ⟦<case>:fix⟧; in the live scheme next to the runs it is followed by the value of the last
// fill, ⟦<case>:bar⟧⟪[████] 100 %  ✅ 200⟫, which the next fill replaces.
var anchor = regexp.MustCompile(`⟦([^⟦⟧]+)⟧(?:⟪[^⟪⟫]*⟫)?`)

// liveValue drops the brackets of a live value from the value itself, so the next fill finds where it ends.
var liveValue = strings.NewReplacer("⟪", "", "⟫", "")

// The words of the anchor grammar and of the filled text.
const (
	anyCase     = "*"
	wordReason  = "reason"
	wordProto   = "protocol"
	wordTrace   = "trace"
	wordNotRun  = "not run"
	wordBar     = "bar"
	wordPrev    = "previous"
	wordFix     = "fix"
	noValue     = "—"
	caseIDBound = " -_:·."
)

// fill replaces the anchors of one scheme file written to the folder from; live keeps every anchor with its value
// after it, ⟦<anchor>⟧⟪<value>⟫, so the file stays fillable by the next run.
type fill struct {
	rows []row
	from string
	live bool
	// past are the earlier runs of the set, the previous state of a case (dynamics.go).
	past []pastRun
}

func (f fill) text(s string) string {
	return anchor.ReplaceAllStringFunc(s, func(a string) string {
		key := anchor.FindStringSubmatch(a)[1]
		v := f.value(strings.Split(key, ":"))

		if !f.live {
			return v
		}

		return "⟦" + key + "⟧⟪" + liveValue.Replace(v) + "⟫"
	})
}

func (f fill) value(parts []string) string {
	if parts[0] == anyCase && len(parts) == 2 {
		return f.allCases(parts[1])
	}

	r := f.find(parts[0])

	switch {
	case len(parts) == 1:
		return f.caseValue(r)

	case len(parts) == 2 && parts[1] == wordProto:
		return f.protocolValue(r)

	case len(parts) == 2 && parts[1] == wordTrace:
		return f.traceValue(r)

	case len(parts) == 2 && parts[1] == wordBar:
		return f.barValue(parts[0])

	case len(parts) == 2 && parts[1] == wordPrev:
		return f.previousValue(r)

	case len(parts) == 2 && parts[1] == wordFix:
		return f.fixValue(r)

	case len(parts) == 3 && parts[2] == wordReason:
		return f.reasonValue(r, parts[1])

	default:
		return f.hopValue(r, parts[1])
	}
}

// find is the testcase of the run named key or whose name, or the name of its testsuite, starts with key and a
// separator (the row id SB-01-02, S-001, GW-B0005); of several, the first failed one, else the first.
func (f fill) find(key string) *row {
	var found *row

	for i := range f.rows {
		r := &f.rows[i]

		if !named(r.testcase, key) && !named(r.suite, key) {
			continue
		}

		if found == nil || (found.view.Mark != venom.MarkFailed && r.view.Mark == venom.MarkFailed) {
			found = r
		}
	}

	return found
}

// named: the testcase name is key, or key and a separator then the rest of the name.
func named(name, key string) bool {
	if name == key {
		return true
	}

	if !strings.HasPrefix(name, key) {
		return false
	}

	next, _ := utf8.DecodeRuneInString(name[len(key):])

	return strings.ContainsRune(caseIDBound, next)
}

// barValue is the progress bar of a node or a case of the scheme drawn at intake, keys its case ids apart by spaces or
// commas: of one testcase the bar of its steps and its verdict, of several the bar of the testcases and passed of all.
func (f fill) barValue(keys string) string {
	var rows []row
	seen := map[int]bool{}

	for _, key := range strings.FieldsFunc(keys, func(r rune) bool { return r == ' ' || r == ',' }) {
		for i := range f.rows {
			if !seen[i] && (named(f.rows[i].testcase, key) || named(f.rows[i].suite, key)) {
				seen[i] = true
				rows = append(rows, f.rows[i])
			}
		}
	}

	if len(rows) == 1 {
		return progress(stepCount(rows[0])) + "  " + verdictOf(rows[0])
	}

	return bar(countOf(rows))
}

func (f fill) caseValue(r *row) string {
	if r == nil {
		return venom.MarkNotRun
	}

	return mark(r.view.Mark, r.view.Code, r.view.TraceID)
}

func (f fill) hopValue(r *row, hop string) string {
	h, ok := hopOf(r, hop)
	if !ok {
		return venom.MarkNotRun
	}

	return mark(h.Mark, h.Code, h.TraceID)
}

func (f fill) reasonValue(r *row, hop string) string {
	h, ok := hopOf(r, hop)

	switch {
	case !ok || h.Mark == venom.MarkNotRun:
		return wordNotRun

	case h.Reason == "":
		return noValue

	default:
		return h.Reason
	}
}

// previousValue is the case against the previous run of the set that has it (dynamicsOf), «—» of a case the run does
// not have.
func (f fill) previousValue(r *row) string {
	if r == nil {
		return noValue
	}

	return layout{past: f.past}.dynamicsOf(*r)
}

// fixValue is the zone of fix responsibility of the case in full (fixDetail), «—» of a case the run does not have.
func (f fill) fixValue(r *row) string {
	if r == nil {
		return noValue
	}

	return fixDetail(*r)
}

func (f fill) protocolValue(r *row) string {
	if r == nil {
		return noValue
	}

	return link(wordProto, f.from, r.protocol)
}

func (f fill) traceValue(r *row) string {
	if r == nil {
		return noValue
	}

	return traceCell(f.from, *r)
}

// allCases is the hop over every testcase that has it: ✅ when all passed, else ❌; then passed of all.
func (f fill) allCases(hop string) string {
	passed, all, failed := 0, 0, false

	for i := range f.rows {
		h, ok := f.rows[i].view.Hops[hop]
		if !ok {
			continue
		}

		all++

		switch h.Mark {
		case venom.MarkPassed:
			passed++

		case venom.MarkFailed:
			failed = true

		default:
		}
	}

	switch {
	case all == 0:
		return venom.MarkNotRun

	case failed:
		return fmt.Sprintf("%s %d/%d", venom.MarkFailed, passed, all)

	default:
		return fmt.Sprintf("%s %d/%d", venom.MarkPassed, passed, all)
	}
}

func hopOf(r *row, hop string) (venom.HopView, bool) {
	if r == nil {
		return venom.HopView{}, false
	}

	h, ok := r.view.Hops[hop]

	return h, ok
}

// mark is ✅/❌/⏸ with the answer code and the first 8 characters of the trace id, each when there is one.
func mark(m, code, traceID string) string {
	words := []string{m}

	if code != "" {
		words = append(words, code)
	}

	if traceID != "" {
		words = append(words, "`"+short(traceID)+"`")
	}

	return strings.Join(words, " ")
}
