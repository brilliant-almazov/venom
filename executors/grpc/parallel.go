package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/ovh/venom"
)

// ParallelResult is the result of a step with `parallel: N`: the first call as a plain step result plus every call in
// call order.
type ParallelResult struct {
	Systemout     string      `json:"systemout,omitempty" yaml:"systemout,omitempty"`
	SystemoutJSON interface{} `json:"systemoutjson,omitempty" yaml:"systemoutjson,omitempty"`
	Systemerr     string      `json:"systemerr,omitempty" yaml:"systemerr,omitempty"`
	Err           string      `json:"err,omitempty" yaml:"err,omitempty"`
	Code          string      `json:"code,omitempty" yaml:"code,omitempty"`
	Status        string      `json:"status,omitempty" yaml:"status,omitempty"`
	Message       string      `json:"message,omitempty" yaml:"message,omitempty"`
	TraceID       string      `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	TraceSaved    string      `json:"trace_saved,omitempty" yaml:"trace_saved,omitempty"`
	TraceFile     string      `json:"trace_file,omitempty" yaml:"trace_file,omitempty"`
	// Codes: the gRPC status name of every call joined by a comma, in call order ("OK,OK,AlreadyExists").
	Codes string `json:"codes" yaml:"codes"`
	// Bodies: the answer of every call as one line (JSON compacted) joined by a newline, in call order (an error call
	// gives its status message).
	Bodies string `json:"bodies" yaml:"bodies"`
	// DistinctBodies: how many different answer texts the calls gave.
	DistinctBodies int `json:"distinct_bodies" yaml:"distinct_bodies"`
	// TraceIDs: the trace id of every call joined by a comma, in call order.
	TraceIDs string `json:"trace_ids" yaml:"trace_ids"`
	// Calls: every call result, in call order.
	Calls []Result `json:"calls" yaml:"calls"`
}

// runParallel sends the same request n times at once: n goroutines wait behind one barrier, each runs the step without
// `parallel`, and the results keep the call order.
func runParallel(ctx context.Context, step venom.TestStep, n int) (interface{}, error) {
	one := venom.TestStep{}
	for k, v := range step {
		if k != "parallel" {
			one[k] = v
		}
	}
	results := make([]Result, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			out, err := Executor{}.Run(ctx, one)
			errs[i] = err
			if r, ok := out.(Result); ok {
				results[i] = r
			} else if err != nil {
				results[i] = Result{Err: err.Error()}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return aggregate(results), nil
}

// aggregate folds the call results of a parallel step into one result.
func aggregate(results []Result) ParallelResult {
	out := ParallelResult{Calls: results}
	if len(results) > 0 {
		r := results[0]
		out.Systemout, out.SystemoutJSON, out.Systemerr, out.Err = r.Systemout, r.SystemoutJSON, r.Systemerr, r.Err
		out.Code, out.Status, out.Message = r.Code, r.Status, r.Message
		out.TraceID, out.TraceSaved, out.TraceFile = r.TraceID, r.TraceSaved, r.TraceFile
	}
	codes := make([]string, len(results))
	bodies := make([]string, len(results))
	traces := make([]string, len(results))
	seen := map[string]struct{}{}
	for i, r := range results {
		codes[i] = r.Status
		body := compact(r.Systemout)
		if r.Status != "" && r.Status != "OK" {
			body = r.Message
		}
		bodies[i] = body
		traces[i] = r.TraceID
		seen[body] = struct{}{}
	}
	out.Codes = strings.Join(codes, ",")
	out.Bodies = strings.Join(bodies, "\n")
	out.DistinctBodies = len(seen)
	out.TraceIDs = strings.Join(traces, ",")
	return out
}

// compact puts a JSON answer on one line; any other text stays as it is.
func compact(s string) string {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}
