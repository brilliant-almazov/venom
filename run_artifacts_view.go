package venom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Reasons of a step or a testcase that did not run.
const (
	ReasonStepNotReached = "not run: the testcase stopped before this step"
	ReasonCaseSkipped    = "skipped"
)

// expectedKeys are the info keys of the expected behaviour, as the protocol prints them.
var expectedKeys = []string{"happens", "expect", "right", "norm"}

// ProtocolOf is the protocol of the testcase, secrets hidden, and the facts a run artifacts writer lays out.
func ProtocolOf(ctx context.Context, ts *TestSuite, tc *TestCase) ProtocolView {
	call, n := protocolLastCall(tc, protocolPresenceRows(tc))
	view := ProtocolView{
		Name:    CaseName(tc),
		Suite:   ts.Name,
		File:    ts.Filepath,
		Line:    tc.SourceLine,
		Text:    HideSensitive(ctx, protocolText(ts, tc)),
		Mark:    protocolMark(tc.Status),
		Code:    protocolCode(call),
		TraceID: protocolTraceID(call),
		Trees:   map[string]string{},
		Reason:  protocolReason(tc),
		Hops:    map[string]HopView{},
	}
	if view.TraceID != "" {
		view.TraceFile, _ = protocolTreeFile(tc, view.TraceID)
	}
	if call != nil {
		view.Request = strings.Trim(protocolRequest(call), "`")
		view.CallStep = protocolStepNumber(call, n)
	}
	info := protocolInfo(tc)
	for _, k := range expectedKeys {
		for _, val := range info[k] {
			view.Expected = append(view.Expected, k+": "+val)
		}
	}
	view.Steps = protocolStepViews(tc)
	for i := range view.Steps {
		view.Steps[i].Body = HideSensitive(ctx, view.Steps[i].Body)
	}
	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		if id := protocolTraceID(r); id != "" {
			if f, _ := protocolTreeFile(tc, id); f != "" {
				view.Trees[id] = f
			}
		}
		for _, key := range protocolHopKeys(r) {
			view.Hops[key] = HopView{Mark: protocolMark(r.Status), Code: protocolCode(r), TraceID: protocolTraceID(r), Reason: protocolStepReason(r)}
		}
	}
	return view
}

func protocolHopKeys(r *TestStepResult) []string {
	var keys []string
	for _, l := range r.ComputedInfo {
		if m := protocolInfoLine.FindStringSubmatch(l); m != nil && m[1] == "hop" && m[2] != "" {
			keys = append(keys, m[2])
		}
	}
	return keys
}

func protocolMark(s Status) string {
	switch s {
	case StatusPass:
		return MarkPassed
	case StatusFail:
		return MarkFailed
	}
	return MarkNotRun
}

func protocolCode(r *TestStepResult) string {
	if r == nil {
		return ""
	}
	for _, k := range []string{"result.statuscode", "result.code"} {
		if val, ok := r.ComputedVars[k]; ok && fmt.Sprint(val) != "" {
			return fmt.Sprint(val)
		}
	}
	return ""
}

// protocolReason is the first failed assertion of the testcase, or its skip reason; empty when it passed.
func protocolReason(tc *TestCase) string {
	if tc.Status == StatusSkip && len(tc.Skipped) > 0 {
		return tc.Skipped[0].Value
	}
	for i := range tc.TestStepResults {
		if s := protocolStepReason(&tc.TestStepResults[i]); s != "" {
			return s
		}
	}
	return ""
}

func protocolStepReason(r *TestStepResult) string {
	for _, e := range r.Errors {
		return strings.Join(strings.Fields(e.Value), " ")
	}
	for _, s := range r.Skipped {
		return strings.Join(strings.Fields(s.Value), " ")
	}
	return ""
}

// PlannedView is the view of a testcase before it runs, or of one the run never reached: its steps as the scenario
// writes them, every one not run. It reads only what the parse of the testsuite set, so it may be taken while the run
// goes on.
func PlannedView(ts *TestSuite, tc *TestCase) ProtocolView {
	view := ProtocolView{Name: CaseName(tc), Suite: ts.Name, File: ts.Filepath, Line: tc.SourceLine, Mark: MarkNotRun,
		Trees: map[string]string{}, Hops: map[string]HopView{}}
	for i := range tc.RawTestSteps {
		view.Steps = append(view.Steps, plannedStep(tc, i))
	}
	return view
}

// protocolStepViews are the steps of a testcase that ran, in file order: one per step result, then every step of the
// scenario the testcase stopped before.
func protocolStepViews(tc *TestCase) []StepView {
	var steps []StepView
	reached := 0
	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		number := protocolStepNumber(r, i)
		if number > reached {
			reached = number
		}
		m := protocolStep(r)
		name := r.Name
		if name == "" {
			name = protocolField(m["name"])
		}
		steps = append(steps, StepView{Number: number, Line: stepLine(tc, number), Name: name,
			Type: protocolField(m["type"]), Request: strings.Trim(protocolRequest(r), "`"), Mark: protocolMark(r.Status),
			Code: protocolCode(r), TraceID: protocolTraceID(r), Reason: protocolStepReason(r), Body: protocolBody(r)})
	}
	if tc.Status == StatusSkip && len(tc.TestStepResults) == 0 {
		for i := range tc.RawTestSteps {
			s := plannedStep(tc, i)
			s.Reason = ReasonCaseSkipped + ": " + protocolReason(tc)
			steps = append(steps, s)
		}
		return steps
	}
	for i := reached; i < len(tc.RawTestSteps); i++ {
		steps = append(steps, plannedStep(tc, i))
	}
	return steps
}

// protocolStepNumber is the 1-based step of a result, its index when the step failed before it was numbered.
func protocolStepNumber(r *TestStepResult, index int) int {
	if r.Number > 0 {
		return r.Number
	}
	return index + 1
}

// plannedStep is step i of the scenario as written, not run.
func plannedStep(tc *TestCase, i int) StepView {
	m := map[string]interface{}{}
	_ = json.Unmarshal(tc.RawTestSteps[i], &m)
	words := []string{protocolField(m["type"])}
	for _, k := range []string{"method", "url", "path", "service", "action"} {
		if v := protocolField(m[k]); v != "" {
			words = append(words, v)
		}
	}
	return StepView{Number: i + 1, Line: stepLine(tc, i+1), Name: protocolField(m["name"]), Type: protocolField(m["type"]),
		Request: strings.TrimSpace(strings.Join(words, " ")), Mark: MarkNotRun, Reason: ReasonStepNotReached}
}

// stepLine is the line of the 1-based step in the testsuite file, the line of the testcase when unknown.
func stepLine(tc *TestCase, number int) int {
	if number >= 1 && number <= len(tc.StepSourceLines) && tc.StepSourceLines[number-1] > 0 {
		return tc.StepSourceLines[number-1]
	}
	return tc.SourceLine
}
