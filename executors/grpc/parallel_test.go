package grpc

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/ovh/venom"
)

// countingHealth answers Check and counts the calls in flight at once.
type countingHealth struct {
	*health.Server
	inFlight, peak int32
}

func (c *countingHealth) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	n := atomic.AddInt32(&c.inFlight, 1)
	defer atomic.AddInt32(&c.inFlight, -1)
	for {
		p := atomic.LoadInt32(&c.peak)
		if n <= p || atomic.CompareAndSwapInt32(&c.peak, p, n) {
			break
		}
	}
	time.Sleep(300 * time.Millisecond)
	return c.Server.Check(ctx, in)
}

func TestRunParallelKeepsCallOrderAndOverlaps(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	h := &countingHealth{Server: health.NewServer()}
	healthpb.RegisterHealthServer(srv, h)
	reflection.Register(srv)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	out, err := Executor{}.Run(context.Background(), venom.TestStep{
		"url":       lis.Addr().String(),
		"method":    "grpc.health.v1.Health/Check",
		"data_json": "{}",
		"trace":     "auto",
		"parallel":  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := out.(ParallelResult)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if r.Codes != "OK,OK,OK,OK,OK" {
		t.Errorf("codes %q", r.Codes)
	}
	if r.DistinctBodies != 1 || len(r.Calls) != 5 || len(strings.Split(r.Bodies, "\n")) != 5 {
		t.Errorf("distinct %d calls %d bodies %q", r.DistinctBodies, len(r.Calls), r.Bodies)
	}
	if ids := strings.Split(r.TraceIDs, ","); len(ids) != 5 || ids[0] == ids[1] || r.TraceID != ids[0] {
		t.Errorf("trace ids %q, first %q", r.TraceIDs, r.TraceID)
	}
	if r.Status != "OK" {
		t.Errorf("first status %q", r.Status)
	}
	if atomic.LoadInt32(&h.peak) < 2 {
		t.Errorf("calls did not overlap: peak %d", h.peak)
	}
}

func TestAggregateCountsErrorMessagesAsBodies(t *testing.T) {
	r := aggregate([]Result{
		{Status: "OK", Systemout: `{"orderId":"1"}`},
		{Status: "AlreadyExists", Message: "taken"},
		{Status: "OK", Systemout: `{"orderId":"1"}`},
	})
	if r.Codes != "OK,AlreadyExists,OK" || r.DistinctBodies != 2 || r.Bodies != "{\"orderId\":\"1\"}\ntaken\n{\"orderId\":\"1\"}" {
		t.Errorf("%+v", r)
	}
}
