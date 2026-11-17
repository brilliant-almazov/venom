package venom

import (
	"context"
	"fmt"
	"strings"
)

// ProtocolOf is the protocol of the testcase, secrets hidden, and the facts a run artifacts writer lays out.
func ProtocolOf(ctx context.Context, ts *TestSuite, tc *TestCase) ProtocolView {
	call, _ := protocolLastCall(tc, protocolPresenceRows(tc))
	view := ProtocolView{
		Name:    CaseName(tc),
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
