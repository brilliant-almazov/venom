package tempo

import (
	"encoding/json"
	"testing"
)

// detailTrace has a server span with no status that answered a failure code with a fault, its payload event, a database
// span with a query and two arguments given out of order, a cache span and a bus span.
const detailTrace = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-store"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"r","name":"store.v1.ItemService/Get","startTimeUnixNano":"1000000","endTimeUnixNano":"9000000","status":{},
   "attributes":[{"key":"rpc.response.status_code","value":{"stringValue":"NOT_FOUND"}},
    {"key":"fault.reason","value":{"stringValue":"unknown_item"}},{"key":"fault.domain","value":{"stringValue":"store.item"}}],
   "events":[{"name":"rpc.payload.received","attributes":[{"key":"rpc.request.body_size","value":{"intValue":"12"}}]}]},
  {"spanId":"q","parentSpanId":"r","name":"query","startTimeUnixNano":"2000000","endTimeUnixNano":"3000000",
   "attributes":[{"key":"db.system.name","value":{"stringValue":"postgresql"}},
    {"key":"db.query.text","value":{"stringValue":"SELECT id\n  FROM item WHERE id = $1 AND kind = $2"}},
    {"key":"db.query.text.arg.2","value":{"stringValue":"book"}},{"key":"db.query.text.arg.1","value":{"intValue":"7"}},
    {"key":"db.rows_affected","value":{"stringValue":"0"}},{"key":"caller","value":{"stringValue":"x.go:1"}}]},
  {"spanId":"c","parentSpanId":"r","name":"get","startTimeUnixNano":"4000000","endTimeUnixNano":"5000000",
   "attributes":[{"key":"db.system","value":{"stringValue":"redis"}},{"key":"server.address","value":{"stringValue":"cache.local"}},
    {"key":"server.port","value":{"intValue":"6379"}},{"key":"db.query.text","value":{"stringValue":"get item:7"}}]},
  {"spanId":"p","parentSpanId":"r","name":"publish","startTimeUnixNano":"6000000","endTimeUnixNano":"7000000",
   "attributes":[{"key":"messaging.system","value":{"stringValue":"rabbitmq"}},
    {"key":"messaging.destination.name","value":{"stringValue":"store.events"}},{"key":"rpc.response.status_code","value":{"stringValue":"OK"}}]}]}]}]}`

func TestRenderDetails(t *testing.T) {
	want := "- svc-store · store.v1.ItemService/Get · 8.0 ms · ❌ unknown_item\n" +
		"  - rpc.response.status_code: NOT_FOUND\n" +
		"  - fault.domain: store.item\n" +
		"  - fault.reason: unknown_item\n" +
		"  - event rpc.payload.received: rpc.request.body_size=12\n" +
		"  - svc-store · query · 1.0 ms · ✅\n" +
		"    - db.system.name: postgresql\n" +
		"    - db.query.text: `SELECT id FROM item WHERE id = $1 AND kind = $2`\n" +
		"    - db.query.text.arg.1: `7`\n" +
		"    - db.query.text.arg.2: `book`\n" +
		"    - db.rows_affected: 0\n" +
		"  - svc-store · get · 1.0 ms · ✅\n" +
		"    - db.system: redis\n" +
		"    - server.address: cache.local:6379\n" +
		"    - db.query.text: `get item:7`\n" +
		"  - svc-store · publish · 1.0 ms · ✅\n" +
		"    - messaging.destination.name: store.events\n" +
		"    - messaging.system: rabbitmq\n"
	if got := Render([]byte(detailTrace)); got != want {
		t.Fatalf("tree\n%s\nwant\n%s", got, want)
	}
}

func TestRenderStatusOfAnAnswerCode(t *testing.T) {
	for code, want := range map[string]string{"INVALID_ARGUMENT": "❌ INVALID_ARGUMENT", "OK": "✅", "0": "✅"} {
		s := renderSpan{Attributes: []renderAttr{{Key: "rpc.response.status_code"}}}
		v := code
		s.Attributes[0].Value.StringValue = &v
		if got := renderStatus(s); got != want {
			t.Fatalf("%s: %q, want %q", code, got, want)
		}
	}
	s := renderSpan{Attributes: []renderAttr{{Key: "http.response.status_code"}}}
	n := []byte(`503`)
	raw := (*json.RawMessage)(&n)
	s.Attributes[0].Value.IntValue = raw
	if got := renderStatus(s); got != "❌ 503" {
		t.Fatalf("http 503: %q", got)
	}
}

func TestCodeSpanFencesBackticks(t *testing.T) {
	if got := codeSpan("a `b` c"); got != "``a `b` c``" {
		t.Fatalf("%q", got)
	}
}
