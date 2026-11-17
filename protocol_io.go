package venom

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// protocolIOFile is the per-call log of a run, one JSON line per call of every testcase, next to the protocols.
const protocolIOFile = "io.jsonl"

// protocolIOMu serializes the lines of testcases and testsuites run in parallel (--parallel, --parallel-suites).
var protocolIOMu sync.Mutex

// protocolIOLine is one call: where it is, what it sent and what it answered. Secrets and credentials are hidden.
type protocolIOLine struct {
	Testsuite       string            `json:"testsuite"`
	File            string            `json:"file"`
	Testcase        string            `json:"testcase"`
	Step            int               `json:"step"`
	Name            string            `json:"name"`
	Executor        string            `json:"executor"`
	Call            string            `json:"call"`
	Method          string            `json:"method,omitempty"`
	URL             string            `json:"url,omitempty"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	Request         interface{}       `json:"request,omitempty"`
	Code            string            `json:"code"`
	TraceID         string            `json:"trace_id,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	Response        string            `json:"response"`
	Status          Status            `json:"status"`
	Errors          []string          `json:"errors,omitempty"`
	Start           time.Time         `json:"start"`
	DurationS       float64           `json:"duration_s"`
}

// writeProtocolIO appends <dir>/io.jsonl with one line per call of the testcase: a step with a code
// (result.statuscode or result.code), whatever its outcome. Run 16-4 of cgw-qa-scenarios had no such log.
func (v *Venom) writeProtocolIO(ctx context.Context, ts *TestSuite, tc *TestCase, dir string) {
	if dir == "" {
		return
	}
	var b strings.Builder
	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		if !protocolHasCode(r) {
			continue
		}
		raw, err := json.Marshal(protocolIOCall(ts, tc, i, r))
		if err != nil {
			Error(ctx, "unable to encode the io line of step %d: %v", i+1, err)
			continue
		}
		b.WriteString(HideSensitive(ctx, string(raw)))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Error(ctx, "unable to create protocol dir %s: %v", dir, err)
		return
	}
	file := filepath.Join(dir, protocolIOFile)
	protocolIOMu.Lock()
	defer protocolIOMu.Unlock()
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		Error(ctx, "unable to open %s: %v", file, err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(b.String()); err != nil {
		Error(ctx, "unable to write %s: %v", file, err)
	}
}

func protocolIOCall(ts *TestSuite, tc *TestCase, i int, r *TestStepResult) protocolIOLine {
	step := protocolStep(r)
	line := protocolIOLine{
		Testsuite: ts.Name,
		File:      ts.Filepath,
		Testcase:  tc.Name,
		Step:      i + 1,
		Name:      r.Name,
		Executor:  protocolField(step["type"]),
		Call:      strings.Trim(protocolRequest(r), "`"),
		TraceID:   protocolTraceID(r),
		Response:  protocolBody(r),
		Status:    r.Status,
		Start:     r.Start,
		DurationS: r.Duration,
	}
	for _, k := range []string{"result.statuscode", "result.code"} {
		if val, ok := r.ComputedVars[k]; ok {
			line.Code = fmt.Sprint(val)
			break
		}
	}
	for _, e := range r.Errors {
		line.Errors = append(line.Errors, e.Value)
	}
	if method, ok := protocolHTTPMethod(r); ok {
		line.Method = fmt.Sprint(method)
		line.URL = fmt.Sprint(r.ComputedVars["result.request.url"])
		line.RequestHeaders = protocolHeaderValues(r, "result.request.header.")
		line.Request = fmt.Sprint(r.ComputedVars["result.request.body"])
		line.ResponseHeaders = protocolHeaderValues(r, "result.headers.")
		return line
	}
	for _, k := range []string{"info", "assertions", "vars", "type"} {
		delete(step, k)
	}
	line.Request = step
	return line
}
