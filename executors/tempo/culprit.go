package tempo

// Culprit is the service and the span of the deepest span of a Tempo trace where the failure arose: the OTLP status
// is error or the gRPC status code is not OK; ok is false when no span of the trace failed.
func Culprit(body []byte) (service, span string, ok bool) {
	t := readTree(body)
	best, depth := -1, -1
	for i, s := range t.spans {
		if !failedSpan(s) {
			continue
		}
		d := 0
		for p := t.parent(i); p >= 0 && d < len(t.spans); p = t.parent(p) {
			d++
		}
		if d > depth {
			best, depth = i, d
		}
	}
	if best < 0 {
		return "", "", false
	}
	return t.spans[best].service, t.spans[best].Name, true
}

// grpcStatusKeys name the gRPC status code of a span: the semantic conventions before and after 1.26.
var grpcStatusKeys = []string{"rpc.grpc.status_code", "rpc.response.status_code"}

// failedSpan: the OTLP status of the span is error, or its gRPC status code is not OK.
func failedSpan(s renderSpan) bool {
	if status(s) != "✅" {
		return true
	}
	for _, k := range grpcStatusKeys {
		if v, ok := s.attr(k); ok && v != "" && v != "0" && v != "OK" {
			return true
		}
	}
	return false
}
