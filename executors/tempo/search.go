package tempo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Search is a Tempo search: Query (TraceQL) or Tags (logfmt, `service.name=a k=v`), the window Start and End (unix
// seconds) and Limit; Spss is the spans per span set of a traceql step.
type Search struct {
	Query string `json:"query" yaml:"query" mapstructure:"query"`
	Tags  string `json:"tags" yaml:"tags" mapstructure:"tags"`
	Start int64  `json:"start" yaml:"start" mapstructure:"start"`
	End   int64  `json:"end" yaml:"end" mapstructure:"end"`
	Limit int    `json:"limit" yaml:"limit" mapstructure:"limit"`
	Spss  int    `json:"spss" yaml:"spss" mapstructure:"spss"`
}

// searchAnswer is the answer of GET /api/search.
type searchAnswer struct {
	Traces []struct {
		TraceID  string      `json:"traceID"`
		SpanSet  *searchSet  `json:"spanSet"`
		SpanSets []searchSet `json:"spanSets"`
	} `json:"traces"`
}

type searchSet struct {
	Spans []struct {
		SpanID   string          `json:"spanID"`
		Name     string          `json:"name"`
		Start    json.RawMessage `json:"startTimeUnixNano"`
		Duration json.RawMessage `json:"durationNanos"`
		Attrs    []renderAttr    `json:"attributes"`
	} `json:"spans"`
}

// Span is one span matched by a traceql step.
type Span struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	Name       string            `json:"name"`
	Start      int64             `json:"start_unix_nano"`
	End        int64             `json:"end_unix_nano"`
	Duration   int64             `json:"duration_nanos"`
	Attributes map[string]string `json:"attributes"`
}

// query asks GET <tempo_url>/api/search with the search and the TraceQL query q.
func (e Executor) query(ctx context.Context, q string) (searchAnswer, error) {
	var a searchAnswer
	s := Search{}
	if e.Search != nil {
		s = *e.Search
	}
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if s.Tags != "" {
		v.Set("tags", s.Tags)
	}
	if s.Start > 0 && s.End > 0 {
		v.Set("start", strconv.FormatInt(s.Start, 10))
		v.Set("end", strconv.FormatInt(s.End, 10))
	}
	if s.Limit > 0 {
		v.Set("limit", strconv.Itoa(s.Limit))
	}
	if s.Spss > 0 {
		v.Set("spss", strconv.Itoa(s.Spss))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.TempoURL+"/api/search?"+v.Encode(), nil)
	if err != nil {
		return a, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return a, fmt.Errorf("tempo: search: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return a, err
	}
	if resp.StatusCode != http.StatusOK {
		return a, fmt.Errorf("tempo: search answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return a, fmt.Errorf("tempo: search answer: %w", err)
	}
	return a, nil
}

// fullID is a trace id of a search answer as 32 hex chars: Tempo drops its leading zeros.
func fullID(id string) string {
	if len(id) < 32 {
		return strings.Repeat("0", 32-len(id)) + id
	}
	return id
}

// search finds the traces of search.query or search.tags, keeps those with a span line that contains an item of
// contains_any (all when it is empty), and takes as the match the first kept trace that holds the checks. The match is
// saved to trace_dir with its tree; result.trace_ids lists the kept traces, result.span_names the span names of the match.
func (e Executor) search(ctx context.Context) (interface{}, error) {
	if e.Search.Query == "" && e.Search.Tags == "" {
		return nil, fmt.Errorf("tempo: search takes query or tags")
	}
	a, err := e.query(ctx, e.Search.Query)
	if err != nil {
		return nil, err
	}
	r := Result{TraceSaved: "no"}
	kept := []string{}
	var match []byte
	firstMismatch := ""
	for _, t := range a.Traces {
		id := fullID(t.TraceID)
		body, ok := fetch(ctx, e.TempoURL, id)
		if !ok {
			continue
		}
		lines := SpanLines(body)
		if !containsAny(lines, e.ContainsAny) {
			continue
		}
		kept = append(kept, id)
		if match != nil {
			continue
		}
		mismatch, err := Mismatch(lines, e.Checks)
		if err != nil {
			return nil, err
		}
		if mismatch == "" {
			match, r.TraceID = body, id
		} else if firstMismatch == "" {
			firstMismatch = mismatch
		}
	}
	r.TraceIDs = jsonList(kept)
	r.Count = len(kept)
	if match == nil {
		r.SpanNames = jsonList(nil)
		if !e.Checks.empty() {
			r.Mismatch = strings.TrimSpace("no found trace holds the checks\n" + firstMismatch)
		}
		return r, nil
	}
	r.Spans = spanCount(match)
	r.SpanNames = jsonList(spanNames(match))
	if e.TraceDir != "" {
		r.TraceFile = filepath.Join(e.TraceDir, r.TraceID+".json")
		if err := mkdirWrite(r.TraceFile, match); err != nil {
			return nil, err
		}
		r.TraceSaved, r.TreeFile = "yes", strings.TrimSuffix(r.TraceFile, ".json")+".md"
	}
	if e.ValuesOf != nil {
		values, err := e.ValuesOf.Of(match)
		if err != nil {
			return nil, err
		}
		r.Values = jsonList(values)
	}
	return r, nil
}

// traceQL sends the TraceQL query as it is and returns every matched span as result.span_list, their number as
// result.count and the traces as result.trace_ids.
func (e Executor) traceQL(ctx context.Context) (interface{}, error) {
	a, err := e.query(ctx, e.TraceQL)
	if err != nil {
		return nil, err
	}
	spans := []Span{}
	ids := []string{}
	for _, t := range a.Traces {
		id := fullID(t.TraceID)
		ids = append(ids, id)
		sets := t.SpanSets
		if len(sets) == 0 && t.SpanSet != nil {
			sets = []searchSet{*t.SpanSet}
		}
		seen := map[string]bool{}
		for _, set := range sets {
			for _, s := range set.Spans {
				if seen[s.SpanID] {
					continue
				}
				seen[s.SpanID] = true
				start, d := nanos(s.Start), nanos(s.Duration)
				attrs := map[string]string{}
				for _, at := range s.Attrs {
					attrs[at.Key] = at.text()
				}
				spans = append(spans, Span{TraceID: id, SpanID: s.SpanID, Name: s.Name, Start: start, End: start + d,
					Duration: d, Attributes: attrs})
			}
		}
	}
	b, err := json.Marshal(spans)
	if err != nil {
		return nil, err
	}
	return Result{TraceSaved: "no", SpanList: string(b), Count: len(spans), TraceIDs: jsonList(ids)}, nil
}

func containsAny(lines, items []string) bool {
	if len(items) == 0 {
		return true
	}
	for _, l := range lines {
		for _, c := range items {
			if strings.Contains(l, c) {
				return true
			}
		}
	}
	return false
}

// spanNames is the span names of a trace, each once, in start order.
func spanNames(body []byte) []string {
	seen := map[string]bool{}
	names := []string{}
	for _, s := range readSpans(body) {
		if !seen[s.Name] {
			seen[s.Name] = true
			names = append(names, s.Name)
		}
	}
	return names
}
