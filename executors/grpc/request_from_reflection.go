package grpc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// requestDepth bounds the nesting of a built request message.
const requestDepth = 8

// wellKnown are the values of the well-known message types, in their JSON form; a type mapped to nil is left out.
var wellKnown = map[string]interface{}{
	"google.protobuf.Timestamp":   "2026-01-01T00:00:00Z",
	"google.protobuf.Duration":    "1s",
	"google.protobuf.StringValue": "qa",
	"google.protobuf.BytesValue":  "cWE=",
	"google.protobuf.BoolValue":   false,
	"google.protobuf.Int32Value":  1,
	"google.protobuf.Int64Value":  "1",
	"google.protobuf.UInt32Value": 1,
	"google.protobuf.UInt64Value": "1",
	"google.protobuf.FloatValue":  1,
	"google.protobuf.DoubleValue": 1,
	"google.protobuf.Struct":      nil,
	"google.protobuf.Value":       nil,
	"google.protobuf.ListValue":   nil,
	"google.protobuf.Any":         nil,
	"google.protobuf.FieldMask":   nil,
	"google.protobuf.Empty":       nil,
}

// requestFromReflection builds the request message of service/method from the descriptor source (server reflection or
// the given proto files) as JSON: BuildRequest of its input type, then data merged over it.
func requestFromReflection(src grpcurl.DescriptorSource, service, method string, fill map[string]string, data []byte) ([]byte, error) {
	d, err := src.FindSymbol(service)
	if err != nil {
		return nil, fmt.Errorf("request_from_reflection: %s: %w", service, err)
	}
	sd, ok := d.(*desc.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("request_from_reflection: %s is not a service", service)
	}
	md := sd.FindMethodByName(method)
	if md == nil {
		return nil, fmt.Errorf("request_from_reflection: %s has no method %s", service, method)
	}
	msg, err := BuildRequest(md.GetInputType(), fill)
	if err != nil {
		return nil, err
	}
	var over map[string]interface{}
	if len(data) > 0 && string(data) != "null" {
		if err := json.Unmarshal(data, &over); err != nil {
			return nil, fmt.Errorf("request_from_reflection: data: %w", err)
		}
	}
	return json.Marshal(merge(msg, over))
}

// BuildRequest is a request message of type md as JSON, keyed by the proto field names: every singular field outside
// a oneof that is not proto3 optional, a value from fill when its path (dot separated) is given — converted to the field
// type — else the first enum value that is not *_UNSPECIFIED, 1 for numbers, "qa" for strings, false for bools; a
// message field is built the same way, a well-known type by its JSON form. Repeated, map, oneof and optional fields
// are left out unless fill names them.
func BuildRequest(md *desc.MessageDescriptor, fill map[string]string) (map[string]interface{}, error) {
	msg := build(md, 0)
	for path, value := range fill {
		if err := setPath(md, msg, strings.Split(path, "."), value); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

func build(md *desc.MessageDescriptor, depth int) map[string]interface{} {
	out := map[string]interface{}{}
	for _, f := range md.GetFields() {
		if f.IsRepeated() || f.IsMap() || f.IsProto3Optional() || f.GetOneOf() != nil {
			continue
		}
		if v, ok := defaultValue(f, depth); ok {
			out[f.GetName()] = v
		}
	}
	return out
}

// defaultValue of a field, false for a field left out.
func defaultValue(f *desc.FieldDescriptor, depth int) (interface{}, bool) {
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		for _, v := range f.GetEnumType().GetValues() {
			if !strings.HasSuffix(v.GetName(), "UNSPECIFIED") {
				return v.GetName(), true
			}
		}
		return nil, false
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		mt := f.GetMessageType()
		if v, ok := wellKnown[mt.GetFullyQualifiedName()]; ok {
			return v, v != nil
		}
		if depth >= requestDepth {
			return nil, false
		}
		return build(mt, depth+1), true
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "qa", true
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "cWE=", true
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return false, true
	case descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "1", true
	}
	return 1, true
}

// setPath puts value at the field path of msg, converted to the type of the last field.
func setPath(md *desc.MessageDescriptor, msg map[string]interface{}, path []string, value string) error {
	f := md.FindFieldByName(path[0])
	if f == nil {
		f = md.FindFieldByJSONName(path[0])
	}
	if f == nil {
		return fmt.Errorf("request_from_reflection: fill: %s has no field %s", md.GetFullyQualifiedName(), path[0])
	}
	if len(path) > 1 {
		mt := f.GetMessageType()
		if mt == nil {
			return fmt.Errorf("request_from_reflection: fill: %s.%s is not a message", md.GetFullyQualifiedName(), f.GetName())
		}
		sub, ok := msg[f.GetName()].(map[string]interface{})
		if !ok {
			sub = map[string]interface{}{}
			msg[f.GetName()] = sub
		}
		return setPath(mt, sub, path[1:], value)
	}
	v, err := convert(f, value)
	if err != nil {
		return err
	}
	msg[f.GetName()] = v
	return nil
}

// convert reads a fill value as the type of f: a number, a bool, an enum name, or JSON for a message or a repeated
// field; 64-bit integers stay strings, as protobuf JSON writes them.
func convert(f *desc.FieldDescriptor, value string) (interface{}, error) {
	bad := func(err error) error {
		return fmt.Errorf("request_from_reflection: fill %s=%q: %w", f.GetName(), value, err)
	}
	if f.IsRepeated() || f.IsMap() || f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		if mt := f.GetMessageType(); !f.IsRepeated() && mt != nil {
			if _, ok := wellKnown[mt.GetFullyQualifiedName()]; ok && !json.Valid([]byte(value)) {
				return value, nil
			}
		}
		var v interface{}
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			return nil, bad(err)
		}
		return v, nil
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, bad(err)
		}
		return b, nil
	case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES,
		descriptorpb.FieldDescriptorProto_TYPE_ENUM,
		descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64, descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return value, nil
	}
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return nil, bad(err)
	}
	return json.Number(value), nil
}

// merge puts over onto base, objects merged key by key.
func merge(base, over map[string]interface{}) map[string]interface{} {
	for k, v := range over {
		if sub, ok := v.(map[string]interface{}); ok {
			if b, ok := base[k].(map[string]interface{}); ok {
				base[k] = merge(b, sub)
				continue
			}
		}
		base[k] = v
	}
	return base
}
