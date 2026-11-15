package tempo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// orderTrace has three consumer/crawl subtrees: in s1 the reserve publish under handle → CreateBySent ends at 10, before
// the first Dial starts at 12; in s2 it ends at 115, after the Dial at 110; s3 has no Dial. The publish p0 is not
// under CreateBySent and ends late in s1.
const orderTrace = `{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-crawler"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"s1","name":"consumer/crawl","startTimeUnixNano":"1","endTimeUnixNano":"50"},
  {"spanId":"h1","parentSpanId":"s1","name":"handle","startTimeUnixNano":"2","endTimeUnixNano":"11"},
  {"spanId":"c1","parentSpanId":"h1","name":"ReserveManager.CreateBySent","startTimeUnixNano":"3","endTimeUnixNano":"11"},
  {"spanId":"p1","parentSpanId":"c1","name":"publish reserve","startTimeUnixNano":"4","endTimeUnixNano":"10"},
  {"spanId":"p0","parentSpanId":"h1","name":"publish reserve","startTimeUnixNano":"5","endTimeUnixNano":"40"},
  {"spanId":"d1","parentSpanId":"s1","name":"DialProvider.Dial","startTimeUnixNano":"12","endTimeUnixNano":"13"},
  {"spanId":"d2","parentSpanId":"s1","name":"DialProvider.Dial","startTimeUnixNano":"20","endTimeUnixNano":"21"},
  {"spanId":"s2","name":"consumer/crawl","startTimeUnixNano":"100","endTimeUnixNano":"150"},
  {"spanId":"h2","parentSpanId":"s2","name":"handle","startTimeUnixNano":"101","endTimeUnixNano":"116"},
  {"spanId":"c2","parentSpanId":"h2","name":"ReserveManager.CreateBySent","startTimeUnixNano":"102","endTimeUnixNano":"116"},
  {"spanId":"p2","parentSpanId":"c2","name":"publish reserve","startTimeUnixNano":"103","endTimeUnixNano":"115"},
  {"spanId":"d3","parentSpanId":"s2","name":"DialProvider.Dial","startTimeUnixNano":"110","endTimeUnixNano":"111"},
  {"spanId":"s3","name":"consumer/crawl","startTimeUnixNano":"200","endTimeUnixNano":"250"},
  {"spanId":"h3","parentSpanId":"s3","name":"handle","startTimeUnixNano":"201","endTimeUnixNano":"211"},
  {"spanId":"c3","parentSpanId":"h3","name":"ReserveManager.CreateBySent","startTimeUnixNano":"202","endTimeUnixNano":"211"},
  {"spanId":"p3","parentSpanId":"c3","name":"publish reserve","startTimeUnixNano":"203","endTimeUnixNano":"210"}]}]}]}`

var reserveOrder = Order{
	Scope:  ` · consumer/crawl · `,
	Before: OrderEnd{Match: ` · publish reserve · `, Parent: `CreateBySent`, Grandparent: ` · handle · `, At: "end"},
	After:  OrderEnd{Match: ` · DialProvider.Dial · `, At: "start", First: true},
}

func TestOrder(t *testing.T) {
	o, err := reserveOrder.Of([]byte(orderTrace))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join([]string{strings.Join(o.Scopes, ","), strings.Join(o.OK, ","), strings.Join(o.Bad, ","),
		strings.Join(o.Missing, ",")}, "|")
	want := "svc-crawler · consumer/crawl · s1,svc-crawler · consumer/crawl · s2,svc-crawler · consumer/crawl · s3|" +
		"svc-crawler · consumer/crawl · s1|" +
		"svc-crawler · consumer/crawl · s2: svc-crawler · publish reserve · p2 at 115 is after " +
		"svc-crawler · DialProvider.Dial · d3 at 110|" +
		"svc-crawler · consumer/crawl · s3"
	if got != want {
		t.Fatalf("scopes|ok|bad|missing\n got %q\nwant %q", got, want)
	}
}

func TestOrderBadInput(t *testing.T) {
	for _, o := range []Order{
		{Scope: "(", Before: reserveOrder.Before, After: reserveOrder.After},
		{Scope: "crawl", Before: OrderEnd{Match: "(", At: "end"}, After: reserveOrder.After},
		{Scope: "crawl", Before: OrderEnd{Match: "publish", Parent: "(", At: "end"}, After: reserveOrder.After},
		{Scope: "crawl", Before: reserveOrder.Before, After: OrderEnd{Match: "Dial", Grandparent: "(", At: "start"}},
		{Scope: "crawl", Before: OrderEnd{Match: "publish", At: "middle"}, After: reserveOrder.After},
		{Scope: "crawl", Before: reserveOrder.Before, After: OrderEnd{Match: "Dial"}},
	} {
		if _, err := o.Of([]byte(orderTrace)); err == nil {
			t.Fatalf("order %+v: a bad regexp or at is an error", o)
		}
	}
}

func TestJoinParent(t *testing.T) {
	j := Join{Left: JoinSide{Match: ` · publish reserve · `, Parent: `CreateBySent`, Attr: "x"},
		Right: JoinSide{Match: `Dial`, Attr: "x"}}
	trace := strings.ReplaceAll(orderTrace, `"name":"publish reserve",`,
		`"name":"publish reserve","attributes":[{"key":"x","value":{"stringValue":"v"}}],`)
	got, err := j.Of([]byte(trace))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Left, ",") != "v,v,v" {
		t.Fatalf("left %q: p0 is not under CreateBySent", got.Left)
	}
	if _, err := (Join{Left: JoinSide{Match: "publish", Parent: "(", Attr: "x"}}).Of([]byte(trace)); err == nil {
		t.Fatal("a bad parent regexp is an error")
	}
}

func TestRunOrdersTheSavedTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(orderTrace)) }))
	defer srv.Close()
	step := map[string]interface{}{"trace_id": "0123456789abcdef0123456789abcdef", "tempo_url": srv.URL, "trace_dir": t.TempDir(),
		"order": map[string]interface{}{
			"scope":  ` · consumer/crawl · `,
			"before": map[string]interface{}{"match": ` · publish reserve · `, "parent": "CreateBySent", "grandparent": ` · handle · `, "at": "end"},
			"after":  map[string]interface{}{"match": ` · DialProvider.Dial · `, "at": "start", "first": true},
		}}
	out, err := Executor{}.Run(context.Background(), step)
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if strings.Count(r.OrderScopes, "\n") != 2 || r.OrderOK != "svc-crawler · consumer/crawl · s1" ||
		!strings.HasPrefix(r.OrderBad, "svc-crawler · consumer/crawl · s2: ") || r.OrderMissing != "svc-crawler · consumer/crawl · s3" {
		t.Fatalf("result %+v", r)
	}
	step["order"] = map[string]interface{}{"scope": "("}
	if _, err := (Executor{}).Run(context.Background(), step); err == nil {
		t.Fatal("a bad regexp is a step error")
	}
}

func TestDefaultAssertionsCheckOrder(t *testing.T) {
	for _, a := range (Executor{}).GetDefaultAssertions().Assertions {
		if a == "result.order_bad ShouldBeEmpty" {
			return
		}
	}
	t.Fatal("result.order_bad ShouldBeEmpty is a default assertion")
}
