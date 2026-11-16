package tempo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// renderEvent is an OTLP span event: rpc.payload.received, rpc.payload.sent, exception and the like.
type renderEvent struct {
	Name       string       `json:"name"`
	Attributes []renderAttr `json:"attributes"`
}

// details are the lines Render prints under a span before its children: the answer code that is not a success, every
// fault.*, for a database or cache span its system, address, query text whole and its arguments in order and
// rows_affected, every messaging.* of a bus span, and every event with its attributes.
func details(s renderSpan) []string {
	var lines []string
	for _, c := range answerCodes {
		if v, ok := s.attr(c.key); ok && !c.ok(v) {
			lines = append(lines, c.key+": "+v)
		}
	}
	lines = append(lines, prefixed(s, "fault.")...)
	if _, ok := s.attr("db.system.name"); ok {
		lines = append(lines, dbDetails(s)...)
	} else if _, ok := s.attr("db.system"); ok {
		lines = append(lines, dbDetails(s)...)
	}
	lines = append(lines, prefixed(s, "messaging.")...)
	for _, e := range s.Events {
		attrs := make([]string, 0, len(e.Attributes))
		for _, a := range e.Attributes {
			attrs = append(attrs, a.Key+"="+a.text())
		}
		lines = append(lines, strings.TrimSpace("event "+e.Name+": "+strings.Join(attrs, ", ")))
	}
	return lines
}

// dbDetails are the system, the address, the query text, the arguments db.query.text.arg.N by N and rows_affected.
func dbDetails(s renderSpan) []string {
	var lines []string
	for _, k := range []string{"db.system.name", "db.system", "db.namespace", "db.operation.name"} {
		if v, ok := s.attr(k); ok {
			lines = append(lines, k+": "+v)
		}
	}
	if host, ok := s.attr("server.address"); ok {
		if port, ok := s.attr("server.port"); ok {
			host += ":" + port
		}
		lines = append(lines, "server.address: "+host)
	}
	for _, k := range []string{"db.query.text", "db.statement"} {
		if v, ok := s.attr(k); ok {
			lines = append(lines, k+": "+codeSpan(v))
		}
	}
	type arg struct {
		n int
		v string
	}
	var args []arg
	for _, a := range s.Attributes {
		if n, err := strconv.Atoi(strings.TrimPrefix(a.Key, "db.query.text.arg.")); err == nil && strings.HasPrefix(a.Key, "db.query.text.arg.") {
			args = append(args, arg{n, a.text()})
		}
	}
	sort.SliceStable(args, func(i, j int) bool { return args[i].n < args[j].n })
	for _, a := range args {
		lines = append(lines, fmt.Sprintf("db.query.text.arg.%d: %s", a.n, codeSpan(a.v)))
	}
	if v, ok := s.attr("db.rows_affected"); ok {
		lines = append(lines, "db.rows_affected: "+v)
	}
	return lines
}
