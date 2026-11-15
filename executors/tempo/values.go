package tempo

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ValuesOf picks attribute values from a trace: Span is a regexp over the span line (SpanLines), Attr over the
// attribute key.
type ValuesOf struct {
	Span string `json:"span" yaml:"span" mapstructure:"span"`
	Attr string `json:"attr" yaml:"attr" mapstructure:"attr"`
}

// Of is the values of the attributes whose key matches Attr, of the spans whose line matches Span, each once, in span
// start order and then attribute key order.
func (v ValuesOf) Of(body []byte) ([]string, error) {
	span, err := regexp.Compile(v.Span)
	if err != nil {
		return nil, fmt.Errorf("tempo: values_of.span %q: %w", v.Span, err)
	}
	attr, err := regexp.Compile(v.Attr)
	if err != nil {
		return nil, fmt.Errorf("tempo: values_of.attr %q: %w", v.Attr, err)
	}
	lines := SpanLines(body)
	seen := map[string]bool{}
	values := []string{}
	for i, s := range readSpans(body) {
		if !span.MatchString(lines[i]) {
			continue
		}
		for _, a := range sortedAttrs(s) {
			if attr.MatchString(a.Key) && !seen[a.text()] {
				seen[a.text()] = true
				values = append(values, a.text())
			}
		}
	}
	return values, nil
}

// jsonList is a list of strings as a JSON array, [] when empty.
func jsonList(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, _ := json.Marshal(items)
	return string(b)
}
