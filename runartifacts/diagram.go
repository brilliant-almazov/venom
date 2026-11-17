package runartifacts

import (
	"fmt"
	"strings"

	"github.com/ovh/venom"
)

// The branches of a scheme drawn in a text block.
const (
	branchMid  = "├─ "
	branchEnd  = "└─ "
	indentMid  = "│  "
	indentEnd  = "   "
	transition = " ─► "
)

// caseNode is the line of a testcase in a scheme: its name and its verdict.
func caseNode(r row) string {
	return fmt.Sprintf("%s · %s", r.testcase, caseMark(r))
}

// caseMark is the verdict of the testcase with the first 8 characters of its trace id.
func caseMark(r row) string {
	if r.state != stateDone {
		return verdictOf(r)
	}

	return mark(r.view.Mark, r.view.Code, r.view.TraceID)
}

// stepNode is the line of a step in a scheme, a transition: number, name, request, then ok / not-ok with the code and
// the trace; the reason of a step that is not ok on the next line.
func stepNode(s venom.StepView) (string, string) {
	words := []string{fmt.Sprint(s.Number)}

	if s.Name != "" {
		words = append(words, s.Name)
	}

	if s.Request != "" {
		words = append(words, "· "+s.Request)
	}

	line := strings.Join(words, " ") + transition + mark(s.Mark, s.Code, s.TraceID)

	if s.Mark == venom.MarkPassed || s.Reason == "" {
		return line, ""
	}

	return line, "reason: " + strings.Join(strings.Fields(s.Reason), " ")
}

// drawSteps draws the steps of a testcase under the prefix of its line.
func drawSteps(b *strings.Builder, prefix string, steps []venom.StepView) {
	for i, s := range steps {
		branch, indent := branchMid, indentMid

		if i == len(steps)-1 {
			branch, indent = branchEnd, indentEnd
		}

		line, reason := stepNode(s)
		b.WriteString(prefix + branch + line + "\n")

		if reason != "" {
			b.WriteString(prefix + indent + "   " + reason + "\n")
		}
	}
}

// drawCases draws the testcases of a path under the prefix, each with its steps.
func drawCases(b *strings.Builder, prefix string, rows []row) {
	for i, r := range rows {
		branch, indent := branchMid, indentMid

		if i == len(rows)-1 {
			branch, indent = branchEnd, indentEnd
		}

		b.WriteString(prefix + branch + caseNode(r) + "\n")
		drawSteps(b, prefix+indent, r.view.Steps)
	}
}
