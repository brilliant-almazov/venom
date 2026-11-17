package tempo

import (
	"regexp"
	"strings"
)

// The directions of a point of a process.
const (
	PointIn  = "in"
	PointOut = "out"
	PointRun = "run"
)

// Point is an entry or an exit of a process seen in a trace: the service of the span, in for an http route, a gRPC
// method or a queue it serves, out for a call, a publish or a table it reads or writes, run for a job with no caller;
// Failed when the span failed.
type Point struct {
	Service, Dir, Name string
	Failed             bool
}

// spanKinds are the OTLP span kinds by number and by name.
var spanKinds = map[string]string{
	"2": kindServer, "3": kindClient, "4": kindProducer, "5": kindConsumer,
	`"SPAN_KIND_SERVER"`: kindServer, `"SPAN_KIND_CLIENT"`: kindClient, `"SPAN_KIND_PRODUCER"`: kindProducer,
	`"SPAN_KIND_CONSUMER"`: kindConsumer,
}

const (
	kindServer   = "server"
	kindClient   = "client"
	kindProducer = "producer"
	kindConsumer = "consumer"
)

// queryTable is the table of an SQL text: FROM, JOIN, INTO, UPDATE.
var queryTable = regexp.MustCompile(`(?i)\b(?:from|join|into|update)\s+("?[a-z_][\w$]*"?(?:\."?[a-z_][\w$]*"?)?)`)

// Points are the entries and exits of every process of a Tempo trace, in start order, each once.
func Points(body []byte) []Point {
	spans := readSpans(body)
	ids := map[string]bool{}
	for _, s := range spans {
		ids[s.SpanID] = true
	}
	var points []Point
	seen := map[Point]bool{}
	for _, s := range spans {
		for _, p := range pointsOf(s, !ids[s.ParentSpanID]) {
			p.Failed = failedSpan(s)
			if !seen[p] {
				seen[p] = true
				points = append(points, p)
			}
		}
	}
	return points
}

// pointsOf are the points of one span; root is a span with no parent in the trace.
func pointsOf(s renderSpan, root bool) []Point {
	at := func(dir, name string) []Point {
		return []Point{{Service: s.service, Dir: dir, Name: name}}
	}
	method, _ := s.attr("http.request.method")
	route, hasRoute := s.attr("http.route")
	rpc, hasRPC := s.attr("rpc.method")
	dest, hasDest := s.attr("messaging.destination.name")
	query, hasQuery := s.attr("db.query.text")
	if service, ok := s.attr("rpc.service"); ok && hasRPC && !strings.Contains(rpc, "/") {
		rpc = service + "/" + rpc
	}
	kind := spanKinds[string(s.Kind)]
	serves := kind == kindServer || kind == "" && root
	switch {
	case hasQuery:
		return tablePoints(s, query)
	case kind == kindConsumer && hasDest:
		return at(PointIn, "queue "+dest)
	case kind == kindProducer && hasDest:
		return at(PointOut, "queue "+dest)
	case serves && hasRPC:
		return at(PointIn, "grpc "+rpc)
	case serves && hasRoute:
		return at(PointIn, "http "+strings.TrimSpace(method+" "+route))
	case kind == kindClient && hasRPC:
		return at(PointOut, "grpc "+rpc)
	case kind == kindClient && method != "":
		return at(PointOut, "http "+strings.TrimSpace(method+" "+hostOf(s)))
	case root:
		return at(PointRun, s.Name)
	}
	return nil
}
