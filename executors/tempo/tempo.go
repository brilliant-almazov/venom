// Package tempo saves a trace from a Grafana Tempo HTTP API to a file, and exposes that as the `tempo` executor.
//
// Next to <trace_dir>/<trace_id>.json the span tree is written as <trace_dir>/<trace_id>.md (Render): the file the
// protocol prints under Trace and the trace gate looks for.
//
// `contains`, `not_contains`, `matches` and `not_matches` check the saved trace with no shell, over one line per span
// "<service> · <span> · <status> · <key>=<value> …" (SpanLines); result.mismatch names each item that does not hold.
// `values_of` gives the attribute values of the matching spans (ValuesOf). `join` pairs the values of two span sets of
// the saved trace (Join, join.go) and result.join_missing names each left value with no right value. `order` checks the
// order of spans in each subtree of the saved trace (Order, order.go) and result.order_bad names each subtree out of
// order.
//
// With `search` and no trace_id the step finds traces by the Tempo search API (search.go); with `traceql` it returns
// the matched spans of a TraceQL query.
//
// A trace is saved when GET <tempo_url>/api/traces/<trace_id> answers 200 with at least one batch (or resourceSpans).
// Save polls every 500 ms up to `wait`; when the trace is not ingested yet, a late fetch keeps polling in the background
// (every 2 s up to LateWait) while the venom process lives, and the result says "late".
package tempo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// Name of the executor.
const Name = "tempo"

// LateWait bounds the background fetch of a trace not ingested within the step wait.
var LateWait = 90 * time.Second

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor saves one trace: trace_id, tempo_url, trace_dir, wait (seconds, default 20), and checks it by contains,
// not_contains, matches, not_matches, join and order; or finds traces by search, or spans by traceql.
type Executor struct {
	TraceID  string `json:"trace_id" yaml:"trace_id" mapstructure:"trace_id"`
	TempoURL string `json:"tempo_url" yaml:"tempo_url" mapstructure:"tempo_url"`
	TraceDir string `json:"trace_dir" yaml:"trace_dir" mapstructure:"trace_dir"`
	Wait     int    `json:"wait" yaml:"wait" mapstructure:"wait"`
	Checks   `mapstructure:",squash"`
	ValuesOf *ValuesOf `json:"values_of" yaml:"values_of" mapstructure:"values_of"`
	Join     *Join     `json:"join" yaml:"join" mapstructure:"join"`
	Order    *Order    `json:"order" yaml:"order" mapstructure:"order"`
	Search   *Search   `json:"search" yaml:"search" mapstructure:"search"`
	// ContainsAny keeps the found traces with a span line that contains one of its items.
	ContainsAny []string `json:"contains_any" yaml:"contains_any" mapstructure:"contains_any"`
	// TraceQL is a TraceQL query sent as it is, over the window of search.
	TraceQL string `json:"traceql" yaml:"traceql" mapstructure:"traceql"`
}

// Result of the executor.
type Result struct {
	TraceID    string `json:"trace_id" yaml:"trace_id"`
	TraceSaved string `json:"trace_saved" yaml:"trace_saved"`
	TraceFile  string `json:"trace_file" yaml:"trace_file"`
	TreeFile   string `json:"tree_file" yaml:"tree_file"`
	Spans      int    `json:"spans" yaml:"spans"`
	Mismatch   string `json:"mismatch" yaml:"mismatch"`
	// Values (a JSON array) is set by values_of.
	Values string `json:"values,omitempty" yaml:"values,omitempty"`
	// TraceIDs and SpanNames (JSON arrays) are set by search; SpanList (a JSON array) and Count by traceql.
	TraceIDs  string `json:"trace_ids,omitempty" yaml:"trace_ids,omitempty"`
	SpanNames string `json:"span_names,omitempty" yaml:"span_names,omitempty"`
	SpanList  string `json:"span_list,omitempty" yaml:"span_list,omitempty"`
	Count     int    `json:"count" yaml:"count"`
	// JoinLeft, JoinRight, JoinPairs and JoinMissing (one value per line) are set by join.
	JoinLeft    string `json:"join_left,omitempty" yaml:"join_left,omitempty"`
	JoinRight   string `json:"join_right,omitempty" yaml:"join_right,omitempty"`
	JoinPairs   string `json:"join_pairs,omitempty" yaml:"join_pairs,omitempty"`
	JoinMissing string `json:"join_missing,omitempty" yaml:"join_missing,omitempty"`
	// OrderScopes, OrderOK, OrderBad and OrderMissing (one item per line) are set by order.
	OrderScopes  string `json:"order_scopes,omitempty" yaml:"order_scopes,omitempty"`
	OrderOK      string `json:"order_ok,omitempty" yaml:"order_ok,omitempty"`
	OrderBad     string `json:"order_bad,omitempty" yaml:"order_bad,omitempty"`
	OrderMissing string `json:"order_missing,omitempty" yaml:"order_missing,omitempty"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: the trace is saved, its checks hold, every left value of join has a right value and every
// subtree of order is in order.
func (Executor) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.trace_saved ShouldEqual yes", "result.mismatch ShouldBeEmpty",
		"result.join_missing ShouldBeEmpty", "result.order_bad ShouldBeEmpty"}}
}

// Run saves the trace.
func (Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	if e.TraceID == "" && e.TraceQL != "" {
		return e.traceQL(ctx)
	}
	if e.TraceID == "" && e.Search != nil {
		return e.search(ctx)
	}
	if !hex32.MatchString(e.TraceID) {
		return nil, fmt.Errorf("tempo: trace_id %q is not 32 lower-case hex", e.TraceID)
	}
	wait := time.Duration(e.Wait) * time.Second
	if e.Wait <= 0 {
		wait = 20 * time.Second
	}
	saved, file, spans := Save(ctx, e.TempoURL, e.TraceDir, e.TraceID, wait)
	r := Result{TraceID: e.TraceID, TraceSaved: saved, TraceFile: file, Spans: spans}
	if saved == "yes" {
		r.TreeFile = strings.TrimSuffix(file, ".json") + ".md"
	}
	if e.Checks.empty() && e.ValuesOf == nil && e.Join == nil && e.Order == nil {
		return r, nil
	}
	if saved != "yes" {
		r.Mismatch = "the trace is not saved, nothing to check"
		return r, nil
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err := e.check(body, &r); err != nil {
		return nil, err
	}
	return r, nil
}

// check sets the mismatch of the checks, the values of values_of, the values of join and the subtrees of order over one
// trace.
func (e Executor) check(body []byte, r *Result) error {
	mismatch, err := Mismatch(SpanLines(body), e.Checks)
	if err != nil {
		return err
	}
	r.Mismatch = mismatch
	if e.ValuesOf != nil {
		values, err := e.ValuesOf.Of(body)
		if err != nil {
			return err
		}
		r.Values = jsonList(values)
	}
	if e.Join != nil {
		j, err := e.Join.Of(body)
		if err != nil {
			return err
		}
		r.JoinLeft, r.JoinRight = strings.Join(j.Left, "\n"), strings.Join(j.Right, "\n")
		r.JoinPairs, r.JoinMissing = strings.Join(j.Pairs, "\n"), strings.Join(j.Missing, "\n")
	}
	if e.Order != nil {
		o, err := e.Order.Of(body)
		if err != nil {
			return err
		}
		r.OrderScopes, r.OrderOK = strings.Join(o.Scopes, "\n"), strings.Join(o.OK, "\n")
		r.OrderBad, r.OrderMissing = strings.Join(o.Bad, "\n"), strings.Join(o.Missing, "\n")
	}
	return nil
}

// NewTraceID returns 32 hex chars of a W3C trace id; a valid given id is kept.
func NewTraceID(given string) string {
	if hex32.MatchString(given) {
		return given
	}
	return randomHex(16)
}

// NewSpanID returns 16 hex chars of a W3C parent span id.
func NewSpanID() string {
	return randomHex(8)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Save fetches the trace into <dir>/<id>.json. It returns "yes" when saved within wait, "late" when a background fetch
// goes on, "no" when there is nothing to save to; the file path; the span count of a saved trace.
func Save(ctx context.Context, tempoURL, dir, id string, wait time.Duration) (string, string, int) {
	if tempoURL == "" || dir == "" {
		return "no", "", 0
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "no", "", 0
	}
	file := filepath.Join(dir, id+".json")
	deadline := time.Now().Add(wait)
	for {
		if body, ok := fetch(ctx, tempoURL, id); ok {
			if err := write(file, body); err == nil {
				return "yes", file, spanCount(body)
			}
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	go func() {
		end := time.Now().Add(LateWait)
		for time.Now().Before(end) {
			time.Sleep(2 * time.Second)
			if body, ok := fetch(context.Background(), tempoURL, id); ok {
				_ = write(file, body)
				return
			}
		}
	}()
	return "late", file, 0
}

func fetch(ctx context.Context, tempoURL, id string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tempoURL+"/api/traces/"+id, nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, false
	}
	return body, spanCount(body) > 0 || batches(body) > 0
}

type traceDoc struct {
	Batches       []batch `json:"batches"`
	ResourceSpans []batch `json:"resourceSpans"`
}

type batch struct {
	ScopeSpans []struct {
		Spans []json.RawMessage `json:"spans"`
	} `json:"scopeSpans"`
	InstrumentationLibrarySpans []struct {
		Spans []json.RawMessage `json:"spans"`
	} `json:"instrumentationLibrarySpans"`
}

func parse(body []byte) traceDoc {
	var d traceDoc
	_ = json.Unmarshal(body, &d)
	return d
}

func batches(body []byte) int {
	d := parse(body)
	return len(d.Batches) + len(d.ResourceSpans)
}

func spanCount(body []byte) int {
	d := parse(body)
	n := 0
	for _, b := range append(d.Batches, d.ResourceSpans...) {
		for _, s := range b.ScopeSpans {
			n += len(s.Spans)
		}
		for _, s := range b.InstrumentationLibrarySpans {
			n += len(s.Spans)
		}
	}
	return n
}

// write puts the trace to file and its span tree next to it as .md.
func write(file string, body []byte) error {
	if err := writeFile(strings.TrimSuffix(file, ".json")+".md", []byte(Render(body))); err != nil {
		return err
	}
	return writeFile(file, body)
}

// mkdirWrite is write into a folder made when missing.
func mkdirWrite(file string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return write(file, body)
}

func writeFile(file string, body []byte) error {
	tmp := file + ".part"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
