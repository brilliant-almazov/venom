package tempo

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// renderDoc is a Tempo trace as the render reads it: the service of each batch and its spans.
type renderDoc struct {
	Batches       []renderBatch `json:"batches"`
	ResourceSpans []renderBatch `json:"resourceSpans"`
}

type renderBatch struct {
	Resource struct {
		Attributes []renderAttr `json:"attributes"`
	} `json:"resource"`
	ScopeSpans                  []renderScope `json:"scopeSpans"`
	InstrumentationLibrarySpans []renderScope `json:"instrumentationLibrarySpans"`
}

type renderScope struct {
	Scope                  renderScopeName `json:"scope"`
	InstrumentationLibrary renderScopeName `json:"instrumentationLibrary"`
	Spans                  []renderSpan    `json:"spans"`
}

type renderScopeName struct {
	Name string `json:"name"`
}

// renderAttr is an OTLP attribute; its value is one of the typed fields, an int given as a JSON string or number.
type renderAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string          `json:"stringValue"`
		IntValue    *json.RawMessage `json:"intValue"`
		DoubleValue *json.Number     `json:"doubleValue"`
		BoolValue   *bool            `json:"boolValue"`
	} `json:"value"`
}

type renderSpan struct {
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Start        json.RawMessage `json:"startTimeUnixNano"`
	End          json.RawMessage `json:"endTimeUnixNano"`
	Attributes   []renderAttr    `json:"attributes"`
	Status       struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"status"`
	service string
	scope   string
}

// readSpans is every span of a Tempo trace with the service of its batch, in start order.
func readSpans(body []byte) []renderSpan {
	var d renderDoc
	if err := json.Unmarshal(body, &d); err != nil {
		return nil
	}
	var spans []renderSpan
	for _, b := range append(d.Batches, d.ResourceSpans...) {
		service := ""
		for _, a := range b.Resource.Attributes {
			if a.Key == "service.name" {
				service = a.text()
			}
		}
		for _, s := range append(b.ScopeSpans, b.InstrumentationLibrarySpans...) {
			scope := s.Scope.Name
			if scope == "" {
				scope = s.InstrumentationLibrary.Name
			}
			for _, sp := range s.Spans {
				sp.service, sp.scope = service, scope
				spans = append(spans, sp)
			}
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return nanos(spans[i].Start) < nanos(spans[j].Start) })
	return spans
}

// Render prints the span tree of a Tempo trace as Markdown, one span per line under its parent, in start order:
// "- <service> · <span> · <ms> ms · ✅" and "❌ <status message>" for a span with an error status. A span whose parent
// is not in the trace is a root.
func Render(body []byte) string {
	spans := readSpans(body)
	ids := map[string]bool{}
	for _, s := range spans {
		ids[s.SpanID] = true
	}
	children := map[string][]renderSpan{}
	for _, s := range spans {
		parent := s.ParentSpanID
		if !ids[parent] || parent == s.SpanID {
			parent = ""
		}
		children[parent] = append(children[parent], s)
	}
	var b strings.Builder
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, s := range children[parent] {
			fmt.Fprintf(&b, "%s- %s · %s · %s ms · %s\n", strings.Repeat("  ", depth), s.service, s.Name,
				strconv.FormatFloat(float64(nanos(s.End)-nanos(s.Start))/1e6, 'f', 1, 64), status(s))
			if s.SpanID != "" {
				walk(s.SpanID, depth+1)
			}
		}
	}
	walk("", 0)
	return b.String()
}

// ScopeKey is the key under which a span line carries the instrumentation scope name of its span.
const ScopeKey = "otel.scope.name"

// SpanLines is one line per span of a Tempo trace, in start order, as contains, not_contains, matches and not_matches
// read it: "<service> · <span> · <status> · <key>=<value> …", the attributes sorted by key, the instrumentation scope
// name among them as otel.scope.name=<scope>.
func SpanLines(body []byte) []string {
	spans := readSpans(body)
	lines := make([]string, 0, len(spans))
	for _, s := range spans {
		attrs := []string{}
		for _, a := range sortedAttrs(s) {
			attrs = append(attrs, a.Key+"="+a.text())
		}
		lines = append(lines, strings.Join(append([]string{s.service, s.Name, status(s)}, attrs...), " · "))
	}
	return lines
}

// sortedAttrs is the attributes of a span sorted by "<key>=<value>", with its instrumentation scope name as ScopeKey.
func sortedAttrs(s renderSpan) []renderAttr {
	attrs := append([]renderAttr{}, s.Attributes...)
	if s.scope != "" {
		scope := s.scope
		a := renderAttr{Key: ScopeKey}
		a.Value.StringValue = &scope
		attrs = append(attrs, a)
	}
	sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].Key+"="+attrs[i].text() < attrs[j].Key+"="+attrs[j].text() })
	return attrs
}

// Checks are the span line checks of a step.
type Checks struct {
	Contains    []string `json:"contains" yaml:"contains" mapstructure:"contains"`
	NotContains []string `json:"not_contains" yaml:"not_contains" mapstructure:"not_contains"`
	Matches     []string `json:"matches" yaml:"matches" mapstructure:"matches"`
	NotMatches  []string `json:"not_matches" yaml:"not_matches" mapstructure:"not_matches"`
}

func (c Checks) empty() bool {
	return len(c.Contains)+len(c.NotContains)+len(c.Matches)+len(c.NotMatches) == 0
}

// Mismatch says what of the checks the span lines break, one item per line; empty when all hold. An item of contains
// must be a substring of some span line, of not_contains of none, a regexp of matches must match some span line, of
// not_matches none.
func Mismatch(lines []string, c Checks) (string, error) {
	contains, notContains, matches := c.Contains, c.NotContains, c.Matches
	var bad []string
	some := func(f func(string) bool) bool {
		for _, l := range lines {
			if f(l) {
				return true
			}
		}
		return false
	}
	for _, c := range contains {
		if !some(func(l string) bool { return strings.Contains(l, c) }) {
			bad = append(bad, "no span contains "+strconv.Quote(c))
		}
	}
	for _, c := range notContains {
		for _, l := range lines {
			if strings.Contains(l, c) {
				bad = append(bad, fmt.Sprintf("span %q contains %q", l, c))
			}
		}
	}
	for _, m := range matches {
		re, err := regexp.Compile(m)
		if err != nil {
			return "", fmt.Errorf("tempo: matches %q: %w", m, err)
		}
		if !some(re.MatchString) {
			bad = append(bad, "no span matches "+strconv.Quote(m))
		}
	}
	for _, m := range c.NotMatches {
		re, err := regexp.Compile(m)
		if err != nil {
			return "", fmt.Errorf("tempo: not_matches %q: %w", m, err)
		}
		for _, l := range lines {
			if re.MatchString(l) {
				bad = append(bad, fmt.Sprintf("span %q matches %q", l, m))
			}
		}
	}
	return strings.Join(bad, "\n"), nil
}

// text is the attribute value as a string.
func (a renderAttr) text() string {
	v := a.Value
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		return strings.Trim(string(*v.IntValue), `"`)
	case v.DoubleValue != nil:
		return v.DoubleValue.String()
	case v.BoolValue != nil:
		return strconv.FormatBool(*v.BoolValue)
	}
	return ""
}

// nanos reads a unix nano time given as a JSON string or number.
func nanos(raw json.RawMessage) int64 {
	n, _ := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	return n
}

// status is ✅, or ❌ with the message for the error status code (2 or STATUS_CODE_ERROR).
func status(s renderSpan) string {
	code := strings.Trim(string(s.Status.Code), `"`)
	if code != "2" && code != "STATUS_CODE_ERROR" {
		return "✅"
	}
	return strings.TrimSpace("❌ " + s.Status.Message)
}
