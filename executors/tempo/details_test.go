package tempo

import (
	"strings"
	"testing"
)

const detailTrace = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-a"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"r","name":"a.v1.ItemService/Get","startTimeUnixNano":"1000000","endTimeUnixNano":"9000000","status":{},
   "attributes":[{"key":"rpc.response.status_code","value":{"stringValue":"NOT_FOUND"}},{"key":"fault.reason","value":{"stringValue":"unknown_item"}},
    {"key":"ok","value":{"boolValue":true}},{"key":"limit","value":{"doubleValue":10}}],
   "events":[{"name":"rpc.payload.received","attributes":[{"key":"rpc.request.body_size","value":{"intValue":"12"}}]}]},
  {"spanId":"q","parentSpanId":"r","name":"query","startTimeUnixNano":"2000000","endTimeUnixNano":"3000000",
   "attributes":[{"key":"db.system.name","value":{"stringValue":"postgresql"}},
    {"key":"db.query.text","value":{"stringValue":"SELECT id FROM item WHERE id = $1 AND kind = $2"}},
    {"key":"db.query.text.arg.1","value":{"intValue":"7"}},{"key":"db.query.text.arg.2","value":{"stringValue":"book"}}]},
  {"spanId":"c","parentSpanId":"r","name":"get","startTimeUnixNano":"4000000","endTimeUnixNano":"5015000",
   "attributes":[{"key":"db.query.text","value":{"stringValue":"SELECT id FROM item WHERE id = $1 AND kind = $2"}}],
   "events":[{"name":"exception","attributes":[{"key":"exception.message","value":{"stringValue":"timeout"}}]}]}]}]}]}`

func TestRenderDetails(t *testing.T) {
	want := strings.Join([]string{
		"└─ svc-a · a.v1.ItemService/Get · 8.0 ms · ❌",
		"   │  problem: grpc NOT_FOUND · fault.reason=unknown_item",
		"   │  fault.reason: unknown_item",
		"   │  limit: 10",
		"   │  ok: True",
		"   │  rpc.response.status_code: NOT_FOUND",
		"   │  event: rpc.payload.received",
		"   │     rpc.request.body_size: 12",
		"   ├─ svc-a · query · 1.0 ms · ✅",
		"   │     db.query.text.arg.1: 7",
		"   │     db.query.text.arg.2: book",
		"   │     db.system.name: postgresql",
		"   │     SQL: SELECT id",
		"   │          FROM item",
		"   │          WHERE id = $1",
		"   │          AND kind = $2",
		"   └─ svc-a · get · 1.01 ms · ❌",
		"         problem: timeout",
		"         event: exception",
		"            exception.message: timeout",
		"         SQL: same SQL as the span above",
	}, "\n")
	got := Render("t", "t.json", []byte(detailTrace))
	if !strings.Contains(got, "summary: ✅ 1 spans · ❌ 2 spans · first problem: svc-a a.v1.ItemService/Get — grpc NOT_FOUND · fault.reason=unknown_item") {
		t.Fatalf("summary\n%s", got)
	}
	if !strings.Contains(got, "```text\n"+want+"\n```\n") {
		t.Fatalf("tree\n%s\nwant\n%s", got, want)
	}
}

func TestSQLLinesWrap(t *testing.T) {
	long := "SELECT " + strings.Repeat("column_name, ", 20) + "x FROM t"
	lines := sqlLines(long)
	if len(lines) != 3 || len([]rune(lines[0])) > sqlWidth || lines[2] != "FROM t" {
		t.Fatalf("%q", lines)
	}
}
