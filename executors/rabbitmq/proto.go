package rabbitmq

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/ovh/venom"
	"github.com/streadway/amqp"
)

func abs(dir string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		if filepath.IsAbs(p) || dir == "" {
			out[i] = p
		} else {
			out[i] = filepath.Join(dir, p)
		}
	}
	return out
}

// protoDecoder turns a protobuf message body into its JSON form by the message type of the routing key.
type protoDecoder struct {
	src   grpcurl.DescriptorSource
	types map[string]string
	def   string
}

// decoder is nil when the step names no protoset or proto_files: bodies are then read as JSON text, as before.
func (e Executor) decoder(workdir string) (*protoDecoder, error) {
	var src grpcurl.DescriptorSource
	var err error
	switch {
	case len(e.Protoset) > 0:
		src, err = grpcurl.DescriptorSourceFromProtoSets(abs(workdir, e.Protoset)...)
	case len(e.ProtoFiles) > 0:
		src, err = grpcurl.DescriptorSourceFromProtoFiles(abs(workdir, e.ImportPaths), e.ProtoFiles...)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: proto descriptors: %w", err)
	}
	return &protoDecoder{src: src, types: e.MessageTypes, def: e.MessageType}, nil
}

// decode returns the JSON form of a body and the message type used; an empty type means the routing key has none.
func (d *protoDecoder) decode(ctx context.Context, msg amqp.Delivery) (map[string]interface{}, string, error) {
	name := d.types[msg.RoutingKey]
	if name == "" {
		name = d.def
	}
	if name == "" {
		return nil, "", nil
	}
	sym, err := d.src.FindSymbol(name)
	if err != nil {
		return nil, name, fmt.Errorf("rabbitmq: message type %q: %w", name, err)
	}
	md, ok := sym.(*desc.MessageDescriptor)
	if !ok {
		return nil, name, fmt.Errorf("rabbitmq: %q is not a message", name)
	}
	m := dynamic.NewMessage(md)
	if err := m.Unmarshal(msg.Body); err != nil {
		return nil, name, fmt.Errorf("rabbitmq: decode %q as %s: %w", msg.RoutingKey, name, err)
	}
	js, err := m.MarshalJSON()
	if err != nil {
		return nil, name, err
	}
	out := map[string]interface{}{}
	if err := venom.JSONUnmarshal(js, &out); err != nil {
		return nil, name, err
	}
	venom.Debug(ctx, "decoded %s as %s: %s", msg.RoutingKey, name, string(js))
	return out, name, nil
}
