package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ovh/venom"
)

func TestParallelSendsAtOnceAndKeepsCallOrder(t *testing.T) {
	var inFlight, peak, calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"selected":1,"closed":1}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"selected":1,"closed":0}}`))
	}))
	defer srv.Close()

	out, err := Executor{}.Run(context.Background(), venom.TestStep{
		"method": "POST", "url": srv.URL + "/tick", "body": "{}", "trace": "auto", "parallel": "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := out.(ParallelResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if r.Codes != "200,200,200" || len(r.Calls) != 3 {
		t.Errorf("codes %q calls %d", r.Codes, len(r.Calls))
	}
	if r.DistinctBodies != 2 || strings.Count(r.Bodies, `"closed":1`) != 1 || strings.Count(r.Bodies, "\n") != 2 {
		t.Errorf("distinct %d bodies %q", r.DistinctBodies, r.Bodies)
	}
	if ids := strings.Split(r.TraceIDs, ","); len(ids) != 3 || ids[0] == ids[1] || r.TraceID != ids[0] {
		t.Errorf("trace ids %q first %q", r.TraceIDs, r.TraceID)
	}
	if atomic.LoadInt32(&peak) < 2 {
		t.Errorf("calls did not overlap: peak %d", peak)
	}
	if n, ok := parallelOf(venom.TestStep{"parallel": 4}); !ok || n != 4 {
		t.Errorf("parallel int %d %v", n, ok)
	}
	if _, ok := parallelOf(venom.TestStep{}); ok {
		t.Error("no parallel read as set")
	}
}
