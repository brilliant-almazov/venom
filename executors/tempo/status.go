package tempo

import (
	"strconv"
	"strings"
)

func (s renderSpan) attr(key string) (string, bool) {
	for _, a := range s.Attributes {
		if a.Key == key {
			return a.text(), true
		}
	}
	return "", false
}

func problem(s renderSpan) string {
	var parts []string
	if status(s) != "✅" {
		parts = append(parts, "OTLP ERROR")
	}
	for _, k := range []string{"http.response.status_code", "http.status_code"} {
		if v, ok := s.attr(k); ok {
			if n, err := strconv.Atoi(v); err == nil && n >= 400 {
				parts = append(parts, "http "+v)
				break
			}
		}
	}
	if v, ok := s.attr("rpc.response.status_code"); ok && v != "" && v != "OK" && v != "0" {
		parts = append(parts, "grpc "+v)
	}
	if len(parts) == 0 && len(messages(s)) == 1 {
		return ""
	}
	seen := map[string]bool{}
	for _, m := range messages(s) {
		if strings.TrimSpace(m) != "" && !seen[m] {
			seen[m] = true
			parts = append(parts, m)
		}
	}
	for _, a := range sortedByKey(s.Attributes) {
		if (a.Key == "fault.reason" || strings.Contains(a.Key, ".error.") && strings.HasSuffix(a.Key, "reason")) && a.text() != "none" {
			parts = append(parts, a.Key+"="+a.text())
		}
	}
	return strings.Join(parts, " · ")
}

func mark(s renderSpan) string {
	if problem(s) != "" {
		return "❌"
	}
	return "✅"
}

func messages(s renderSpan) []string {
	out := []string{s.Status.Message}
	for _, e := range s.Events {
		if e.Name != "exception" {
			continue
		}
		for _, a := range e.Attributes {
			if a.Key == "exception.message" {
				out = append(out, a.text())
			}
		}
	}
	return out
}
