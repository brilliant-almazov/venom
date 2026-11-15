package tempo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// joinTrace has two span sets: Dial spans of svc-crawler carry the unit inside arg.req, INSERT spans of svc-store
// carry it as arg.3; u-1 is on both sides, u-2 only on the left (its arg.3 is on a SELECT span).
const joinTrace = `{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-crawler"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"a","name":"Dial","startTimeUnixNano":"1","endTimeUnixNano":"2","attributes":[{"key":"arg.req","value":{"stringValue":"{\"unit\":\"u-1\"}"}}]},
  {"spanId":"b","name":"Dial","startTimeUnixNano":"3","endTimeUnixNano":"4","attributes":[{"key":"arg.req","value":{"stringValue":"{\"unit\":\"u-2\"}"}}]}]}]},
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-store"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"c","name":"INSERT INTO crawl.unit","startTimeUnixNano":"5","endTimeUnixNano":"6","attributes":[{"key":"arg.3","value":{"stringValue":"u-1"}}]},
  {"spanId":"d","name":"SELECT 1","startTimeUnixNano":"7","endTimeUnixNano":"8","attributes":[{"key":"arg.3","value":{"stringValue":"u-2"}}]}]}]}]}`

var unitJoin = Join{
	Left:  JoinSide{Match: ` · Dial · `, Attr: "arg.req", Extract: `"unit":"([^"]+)"`},
	Right: JoinSide{Match: `INSERT INTO`, Attr: "arg.3"},
}

func TestJoin(t *testing.T) {
	j, err := unitJoin.Of([]byte(joinTrace))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join([]string{strings.Join(j.Left, ","), strings.Join(j.Right, ","), strings.Join(j.Pairs, ","),
		strings.Join(j.Missing, ",")}, "|")
	if got != "u-1,u-2|u-1|u-1|u-2" {
		t.Fatalf("left|right|pairs|missing %q", got)
	}
}

func TestJoinBadRegexp(t *testing.T) {
	for _, j := range []Join{
		{Left: JoinSide{Match: "(", Attr: "arg.req"}, Right: unitJoin.Right},
		{Left: unitJoin.Left, Right: JoinSide{Match: "INSERT", Attr: "arg.3", Extract: "("}},
		{Left: JoinSide{Match: "Dial", Attr: "arg.req", Extract: "unit"}, Right: unitJoin.Right},
	} {
		if _, err := j.Of([]byte(joinTrace)); err == nil {
			t.Fatalf("join %+v: a bad regexp is an error", j)
		}
	}
}

func TestRunJoinsTheSavedTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(joinTrace)) }))
	defer srv.Close()
	step := map[string]interface{}{"trace_id": "0123456789abcdef0123456789abcdef", "tempo_url": srv.URL, "trace_dir": t.TempDir(),
		"join": map[string]interface{}{
			"left":  map[string]interface{}{"match": ` · Dial · `, "attr": "arg.req", "extract": `"unit":"([^"]+)"`},
			"right": map[string]interface{}{"match": `INSERT INTO`, "attr": "arg.3"},
		}}
	out, err := Executor{}.Run(context.Background(), step)
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if r.JoinLeft != "u-1\nu-2" || r.JoinRight != "u-1" || r.JoinPairs != "u-1" || r.JoinMissing != "u-2" {
		t.Fatalf("result %+v", r)
	}
	step["join"] = map[string]interface{}{"left": map[string]interface{}{"match": "(", "attr": "arg.req"}}
	if _, err := (Executor{}).Run(context.Background(), step); err == nil {
		t.Fatal("a bad regexp is a step error")
	}
}

func TestDefaultAssertionsCheckJoin(t *testing.T) {
	for _, a := range (Executor{}).GetDefaultAssertions().Assertions {
		if a == "result.join_missing ShouldBeEmpty" {
			return
		}
	}
	t.Fatal("result.join_missing ShouldBeEmpty is a default assertion")
}
