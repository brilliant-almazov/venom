package tempo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ovh/venom/internal/golden"
)

const scopeTrace = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-crawler"}}]},
 "scopeSpans":[
  {"scope":{"name":"internal/order/manager"},"spans":[
   {"spanId":"a","name":"OrderManager.Create","startTimeUnixNano":"1","endTimeUnixNano":"5"},
   {"spanId":"b","parentSpanId":"a","name":"INSERT INTO outbox.outbox_slice","startTimeUnixNano":"2","endTimeUnixNano":"3","attributes":[
    {"key":"db.query.text.arg.2","value":{"stringValue":"ct0"}},{"key":"db.query.text.arg.4","value":{"stringValue":"pending"}}]},
   {"spanId":"c","parentSpanId":"a","name":"INSERT INTO outbox.outbox_slice","startTimeUnixNano":"3","endTimeUnixNano":"4","attributes":[
    {"key":"db.query.text.arg.2","value":{"stringValue":"ct1"}},{"key":"db.query.text.arg.4","value":{"stringValue":"sent"}}]}]},
  {"instrumentationLibrary":{"name":"shared.db"},"spans":[
   {"spanId":"d","parentSpanId":"a","name":"pgx.Query","startTimeUnixNano":"4","endTimeUnixNano":"5"}]}]}]}`

func TestSpanLinesCarryScope(t *testing.T) {
	golden.Text(t, "span_lines_scope", strings.Join(SpanLines([]byte(scopeTrace)), "\n")+"\n")
}

func TestNotMatches(t *testing.T) {
	lines := SpanLines([]byte(scopeTrace))
	m, err := Mismatch(lines, Checks{NotMatches: []string{`otel\.scope\.name=\(?\*?(shared|helpers|utils|common)[.)]`,
		`arg\.2=ct0 · db\.query\.text\.arg\.4=(?:[^p]|p[^e])`}})
	if err != nil {
		t.Fatal(err)
	}
	golden.Text(t, "not_matches", m+"\n")
	if _, err := Mismatch(lines, Checks{NotMatches: []string{"("}}); err == nil {
		t.Fatal("a bad regexp is an error")
	}
}

func TestValuesOf(t *testing.T) {
	values, err := ValuesOf{Span: `INSERT INTO outbox\.outbox_slice`, Attr: `^db\.query\.text\.arg\.4$`}.Of([]byte(scopeTrace))
	if err != nil {
		t.Fatal(err)
	}
	golden.Text(t, "values_of", jsonList(values)+"\n")
	if _, err := (ValuesOf{Span: "(", Attr: "x"}).Of([]byte(scopeTrace)); err == nil {
		t.Fatal("a bad regexp is an error")
	}
}

// tempoServer answers /api/search with search and /api/traces/<id> with the trace of that id.
func tempoServer(t *testing.T, search string, traces map[string]string, query *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			*query = r.URL.RawQuery
			_, _ = w.Write([]byte(search))
			return
		}
		body, ok := traces[strings.TrimPrefix(r.URL.Path, "/api/traces/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearch(t *testing.T) {
	other := `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-crawler"}}]},
 "scopeSpans":[{"spans":[{"spanId":"x","name":"Other","startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`
	var query string
	srv := tempoServer(t, `{"traces":[{"traceID":"23456789abcdef0123456789abcdef"},{"traceID":"0123456789abcdef0123456789abcdee"},{"traceID":"ffffffffffffffffffffffffffffffff"}]}`,
		map[string]string{"0023456789abcdef0123456789abcdef": other, "0123456789abcdef0123456789abcdee": scopeTrace, "ffffffffffffffffffffffffffffffff": scopeTrace}, &query)
	defer srv.Close()
	dir := t.TempDir()
	out, err := Executor{}.Run(context.Background(), map[string]interface{}{"tempo_url": srv.URL, "trace_dir": dir,
		"search":       map[string]interface{}{"tags": "service.name=svc-crawler scenario_id=s1", "start": "1700000000", "end": 1700003600, "limit": 20},
		"contains_any": []string{"ct0", "ct9"}, "contains": []string{"OrderManager.Create"},
		"values_of": map[string]interface{}{"span": "outbox_slice", "attr": `arg\.4$`}})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if _, err := os.Stat(r.TreeFile); err != nil || r.TraceSaved != "yes" {
		t.Fatalf("tree %q saved %q: %v", r.TreeFile, r.TraceSaved, err)
	}
	r.TraceFile, r.TreeFile = "", ""
	golden.JSON(t, "search", map[string]interface{}{"query": query, "result": r})
}

func TestSearchNoMatch(t *testing.T) {
	var query string
	srv := tempoServer(t, `{"traces":[{"traceID":"0123456789abcdef0123456789abcdee"}]}`,
		map[string]string{"0123456789abcdef0123456789abcdee": scopeTrace}, &query)
	defer srv.Close()
	out, err := Executor{}.Run(context.Background(), map[string]interface{}{"tempo_url": srv.URL,
		"search": map[string]interface{}{"query": `{resource.service.name="svc-crawler"}`}, "contains": []string{"Missing"}})
	if err != nil {
		t.Fatal(err)
	}
	golden.JSON(t, "search_no_match", map[string]interface{}{"query": query, "result": out})
	if _, err := (Executor{}).Run(context.Background(), map[string]interface{}{"tempo_url": srv.URL, "search": map[string]interface{}{}}); err == nil {
		t.Fatal("a search with neither query nor tags is an error")
	}
}

func TestTraceQL(t *testing.T) {
	var query string
	srv := tempoServer(t, `{"traces":[{"traceID":"abc","spanSets":[{"spans":[
  {"spanID":"s1","name":"ReserveManager.CreateBySent","startTimeUnixNano":"1000","durationNanos":"500","attributes":[{"key":"unit","value":{"stringValue":"u1"}}]},
  {"spanID":"s2","name":"DialProvider.Dial","startTimeUnixNano":"1600","durationNanos":"100"}]},
 {"spans":[{"spanID":"s2","name":"DialProvider.Dial","startTimeUnixNano":"1600","durationNanos":"100"}]}]}]}`, nil, &query)
	defer srv.Close()
	out, err := Executor{}.Run(context.Background(), map[string]interface{}{"tempo_url": srv.URL,
		"traceql": `{name="ReserveManager.CreateBySent"} >> {name="DialProvider.Dial"}`,
		"search":  map[string]interface{}{"start": 1700000000, "end": 1700003600, "spss": 50}})
	if err != nil {
		t.Fatal(err)
	}
	golden.JSON(t, "traceql", map[string]interface{}{"query": query, "result": out})
}
