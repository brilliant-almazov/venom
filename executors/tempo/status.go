package tempo

import "strconv"

// answerCodes are the attributes a span carries the code of its answer under, with the codes that mean success.
var answerCodes = []struct {
	key string
	ok  func(string) bool
}{
	{"rpc.response.status_code", func(v string) bool { return v == "" || v == "OK" || v == "0" }},
	{"rpc.grpc.status_code", func(v string) bool { return v == "" || v == "0" }},
	{"http.response.status_code", httpOK},
	{"http.status_code", httpOK},
}

func httpOK(v string) bool {
	n, err := strconv.Atoi(v)
	return err != nil || n < 400
}

// attr is the value of the attribute key of a span and whether the span has it.
func (s renderSpan) attr(key string) (string, bool) {
	for _, a := range s.Attributes {
		if a.Key == key {
			return a.text(), true
		}
	}
	return "", false
}

// failure is what makes a span failed beyond its status: its fault.reason, else its answer code that is not a success,
// else its first fault.* as "<key>: <value>";
// empty for a span that carries neither.
func (s renderSpan) failure() string {
	if r, ok := s.attr("fault.reason"); ok && r != "" {
		return r
	}
	for _, c := range answerCodes {
		if v, ok := s.attr(c.key); ok && !c.ok(v) {
			return v
		}
	}
	if f := prefixed(s, "fault."); len(f) > 0 {
		return f[0]
	}
	return ""
}

// renderStatus is the status Render prints: status (✅, or ❌ with the message of an error status), and ❌ with the
// failure for a span whose answer code is not a success or that carries a fault.* attribute.
func renderStatus(s renderSpan) string {
	if st := status(s); st != "✅" {
		return st
	}
	if f := s.failure(); f != "" {
		return "❌ " + f
	}
	return "✅"
}
