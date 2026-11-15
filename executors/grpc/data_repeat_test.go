package grpc

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/ovh/venom"
	"github.com/ovh/venom/internal/golden"
)

func TestDataRepeatExpand(t *testing.T) {
	out, err := DataRepeat{Path: "order.items", Count: 3, Template: `{"id":"ent{{.index}}g","text":"k{{.index}}"}`}.
		Expand([]byte(`{"source":"s1","order":{"id":7}}`))
	if err != nil {
		t.Fatal(err)
	}
	golden.Text(t, "data_repeat_expand", string(out)+"\n")
	for _, bad := range []DataRepeat{{Count: 1, Template: "{}"}, {Path: "a", Count: 1, Template: "{{"}, {Path: "a", Count: 1, Template: "x{{.index}}"}} {
		if _, err := bad.Expand(nil); err == nil {
			t.Errorf("%+v: no error", bad)
		}
	}
}

// TestDataRepeatRun sends the expanded request to a server that accepts any method, and checks the result keeps the
// size and the sha256 of the request and not the request.
func TestDataRepeatRun(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var got int64
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ interface{}, stream grpc.ServerStream) error {
		var in emptypb.Empty
		if err := stream.RecvMsg(&in); err != nil {
			return err
		}
		atomic.StoreInt64(&got, int64(proto.Size(&in)))
		return stream.SendMsg(&emptypb.Empty{})
	}))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Executor{}.Run(context.Background(), venom.TestStep{
		"url":          lis.Addr().String(),
		"method":       "demo.v1.Orders/Place",
		"proto_files":  []string{"orders.proto"},
		"import_paths": []string{dir},
		"data_json":    `{"source":"s1"}`,
		"data_repeat":  map[string]interface{}{"path": "items", "count": 1000, "template": `{"id":"ent{{.index}}g","text":"kw"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if r.Status != "OK" || r.Request != nil || atomic.LoadInt64(&got) < 1000*8 {
		t.Fatalf("status %q request %v server got %d bytes", r.Status, r.Request, got)
	}
	golden.JSON(t, "data_repeat_run", map[string]interface{}{"request_size": r.RequestSize, "request_sha256": r.RequestSHA256})
}
