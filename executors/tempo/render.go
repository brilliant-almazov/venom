package tempo

import (
	"encoding/json"
	"fmt"
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
		Attributes []struct {
			Key   string `json:"key"`
			Value struct {
				StringValue string `json:"stringValue"`
			} `json:"value"`
		} `json:"attributes"`
	} `json:"resource"`
	ScopeSpans                  []renderScope `json:"scopeSpans"`
	InstrumentationLibrarySpans []renderScope `json:"instrumentationLibrarySpans"`
}

type renderScope struct {
	Spans []renderSpan `json:"spans"`
}

type renderSpan struct {
	SpanID       string          `json:"spanId"`
	ParentSpanID string          `json:"parentSpanId"`
	Name         string          `json:"name"`
	Start        json.RawMessage `json:"startTimeUnixNano"`
	End          json.RawMessage `json:"endTimeUnixNano"`
	Status       struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"status"`
	service string
}

// Render prints the span tree of a Tempo trace as Markdown, one span per line under its parent, in start order:
// "- <service> · <span> · <ms> ms · ✅" and "❌ <status message>" for a span with an error status. A span whose parent
// is not in the trace is a root.
func Render(body []byte) string {
	var d renderDoc
	if err := json.Unmarshal(body, &d); err != nil {
		return ""
	}
	var spans []renderSpan
	for _, b := range append(d.Batches, d.ResourceSpans...) {
		service := ""
		for _, a := range b.Resource.Attributes {
			if a.Key == "service.name" {
				service = a.Value.StringValue
			}
		}
		for _, s := range append(b.ScopeSpans, b.InstrumentationLibrarySpans...) {
			for _, sp := range s.Spans {
				sp.service = service
				spans = append(spans, sp)
			}
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return nanos(spans[i].Start) < nanos(spans[j].Start) })
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
