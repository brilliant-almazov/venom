package rabbitmq

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/ovh/venom"
	"github.com/streadway/amqp"
)

func TestDecodeByRoutingKey(t *testing.T) {
	venom.InitTestLogger(t)
	dir := t.TempDir()
	src := "syntax = \"proto3\";\npackage demo.v1;\nmessage Fact { int64 message_id = 1; string kind = 2; }\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.proto"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	dec, err := Executor{ProtoFiles: []string{"demo.proto"}, ImportPaths: []string{"."},
		MessageTypes: map[string]string{"fact": "demo.v1.Fact"}}.decoder(dir)
	if err != nil || dec == nil {
		t.Fatalf("decoder: %v", err)
	}
	sym, err := dec.src.FindSymbol("demo.v1.Fact")
	if err != nil {
		t.Fatal(err)
	}
	m := dynamic.NewMessage(sym.(*desc.MessageDescriptor))
	m.SetFieldByName("message_id", int64(42))
	m.SetFieldByName("kind", "order.accepted")
	body, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	js, name, err := dec.decode(context.Background(), amqp.Delivery{RoutingKey: "fact", Body: body})
	if err != nil || name != "demo.v1.Fact" {
		t.Fatalf("decode: %v %q", err, name)
	}
	if js["messageId"] != "42" || js["kind"] != "order.accepted" {
		t.Fatalf("fields: %v", js)
	}

	if js, name, err = dec.decode(context.Background(), amqp.Delivery{RoutingKey: "other", Body: body}); err != nil || name != "" || js != nil {
		t.Fatalf("a key without a type stays undecoded: %v %q %v", js, name, err)
	}
}

func TestNoDecoderWithoutDescriptors(t *testing.T) {
	dec, err := Executor{}.decoder(t.TempDir())
	if err != nil || dec != nil {
		t.Fatalf("no descriptors, no decoder: %v %v", dec, err)
	}
}
