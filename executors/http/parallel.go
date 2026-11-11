package http

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/ovh/venom"
)

// ParallelResult is the result of an http step with `parallel: N`: the first call as the plain fields plus every call in
// call order.
type ParallelResult struct {
	StatusCode int         `json:"statuscode,omitempty" yaml:"statuscode,omitempty"`
	Body       string      `json:"body,omitempty" yaml:"body,omitempty"`
	BodyJSON   interface{} `json:"bodyjson,omitempty" yaml:"bodyjson,omitempty"`
	Headers    Headers     `json:"headers,omitempty" yaml:"headers,omitempty"`
	Err        string      `json:"err,omitempty" yaml:"err,omitempty"`
	TraceID    string      `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	TraceSaved string      `json:"trace_saved,omitempty" yaml:"trace_saved,omitempty"`
	TraceFile  string      `json:"trace_file,omitempty" yaml:"trace_file,omitempty"`
	// Codes: the HTTP status of every call joined by a comma, in call order ("200,200,409").
	Codes string `json:"codes" yaml:"codes"`
	// Bodies: the answer body of every call on one line joined by a newline, in call order.
	Bodies string `json:"bodies" yaml:"bodies"`
	// DistinctBodies: how many different answer bodies the calls gave.
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

// aggregate folds the call results of a parallel http step into one result.
func aggregate(results []Result) ParallelResult {
	out := ParallelResult{Calls: results}
	if len(results) > 0 {
		r := results[0]
		out.StatusCode, out.Body, out.BodyJSON, out.Headers, out.Err = r.StatusCode, r.Body, r.BodyJSON, r.Headers, r.Err
		out.TraceID, out.TraceSaved, out.TraceFile = r.TraceID, r.TraceSaved, r.TraceFile
	}
	codes := make([]string, len(results))
	bodies := make([]string, len(results))
	traces := make([]string, len(results))
	seen := map[string]struct{}{}
	for i, r := range results {
		codes[i] = strconv.Itoa(r.StatusCode)
		body := strings.ReplaceAll(strings.TrimSpace(r.Body), "\n", " ")
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

// parallelOf reads `parallel` as a number or as the text of a number (a templated value of a user executor).
func parallelOf(step venom.TestStep) (int, bool) {
	switch v := step["parallel"].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	}
	return 0, false
}
