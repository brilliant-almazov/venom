package tempo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

const trace = `{"batches":[{"scopeSpans":[{"spans":[{"spanId":"a"},{"spanId":"b"}]}]}]}`

func TestSaveWaitsForIngestion(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/api/traces/0123456789abcdef0123456789abcdef" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(trace))
	}))
	defer srv.Close()
	dir := t.TempDir()
	saved, file, spans := Save(context.Background(), srv.URL, dir, "0123456789abcdef0123456789abcdef", 5*time.Second)
	if saved != "yes" || spans != 2 {
		t.Fatalf("saved %s spans %d", saved, spans)
	}
	if file != filepath.Join(dir, "0123456789abcdef0123456789abcdef.json") {
		t.Fatalf("file %s", file)
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != trace {
		t.Fatalf("body %q err %v", body, err)
	}
}

func TestSaveLateWhenNotIngested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"batches":[]}`))
	}))
	defer srv.Close()
	saved, _, _ := Save(context.Background(), srv.URL, t.TempDir(), "0123456789abcdef0123456789abcdef", 600*time.Millisecond)
	if saved != "late" {
		t.Fatalf("saved %s", saved)
	}
}

func TestNewTraceID(t *testing.T) {
	if got := NewTraceID("0123456789abcdef0123456789abcdef"); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("kept id %s", got)
	}
	got := NewTraceID("auto")
	if !hex32.MatchString(got) || got == NewTraceID("auto") {
		t.Fatalf("new id %s", got)
	}
	if len(NewSpanID()) != 16 {
		t.Fatal("span id length")
	}
}

const treeTrace = `{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"nginx-gateway"}}]},
 "scopeSpans":[{"spans":[{"spanId":"r","name":"POST /v1/billing/accounts","startTimeUnixNano":"1000000","endTimeUnixNano":"13000000"}]}]},
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-billing"}}]},
 "scopeSpans":[{"spans":[
  {"spanId":"b","parentSpanId":"a","name":"Insert","startTimeUnixNano":"3000000","endTimeUnixNano":"4000000","status":{"code":"STATUS_CODE_ERROR","message":"duplicate key"}},
  {"spanId":"a","parentSpanId":"r","name":"CreateAccount","startTimeUnixNano":"2000000","endTimeUnixNano":"12000000","status":{"code":0}}]}]}]}`

func TestRenderTree(t *testing.T) {
	want := "- nginx-gateway · POST /v1/billing/accounts · 12.0 ms · ✅\n" +
		"  - svc-billing · CreateAccount · 10.0 ms · ✅\n" +
		"    - svc-billing · Insert · 1.0 ms · ❌ duplicate key\n"
	if got := Render([]byte(treeTrace)); got != want {
		t.Fatalf("tree\n%s\nwant\n%s", got, want)
	}
}

func TestSaveWritesTree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(treeTrace)) }))
	defer srv.Close()
	dir := t.TempDir()
	out, err := Executor{}.Run(context.Background(), map[string]interface{}{"trace_id": "0123456789abcdef0123456789abcdef", "tempo_url": srv.URL, "trace_dir": dir})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if r.TreeFile != filepath.Join(dir, "0123456789abcdef0123456789abcdef.md") {
		t.Fatalf("tree file %q", r.TreeFile)
	}
	body, err := os.ReadFile(r.TreeFile)
	if err != nil || string(body) != Render([]byte(treeTrace)) {
		t.Fatalf("tree %q err %v", body, err)
	}
}
