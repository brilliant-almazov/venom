package tempo

import (
	"fmt"
	"regexp"
)

// OrderEnd picks the spans of one end of an order inside a scope: each span whose line (SpanLines) matches Match, whose
// parent's line matches Parent and whose grandparent's line matches Grandparent when those are given. At is the time
// of the span compared, "start" or "end"; with First only the span that starts first is kept.
type OrderEnd struct {
	Match       string `json:"match" yaml:"match" mapstructure:"match"`
	Parent      string `json:"parent" yaml:"parent" mapstructure:"parent"`
	Grandparent string `json:"grandparent" yaml:"grandparent" mapstructure:"grandparent"`
	At          string `json:"at" yaml:"at" mapstructure:"at"`
	First       bool   `json:"first" yaml:"first" mapstructure:"first"`
}

// Order checks the order of spans in each subtree of a saved trace, with no TraceQL: in the subtree of each span whose
// line matches Scope, every Before span is at or before every After span.
type Order struct {
	Scope  string   `json:"scope" yaml:"scope" mapstructure:"scope"`
	Before OrderEnd `json:"before" yaml:"before" mapstructure:"before"`
	After  OrderEnd `json:"after" yaml:"after" mapstructure:"after"`
}

// Ordered is an order over one trace, one item per scope span in start order: Scopes all of them, OK those in order,
// Bad those out of order with the two spans, Missing those with no Before or no After span.
type Ordered struct {
	Scopes, OK, Bad, Missing []string
}

// orderEnd is an OrderEnd with its regexps compiled.
type orderEnd struct {
	match, parent, grandparent *regexp.Regexp
	end, first                 bool
}

// Of checks the order over one trace.
func (o Order) Of(body []byte) (Ordered, error) {
	scope, err := regexp.Compile(o.Scope)
	if err != nil {
		return Ordered{}, fmt.Errorf("tempo: order.scope %q: %w", o.Scope, err)
	}
	before, err := o.Before.compile("before")
	if err != nil {
		return Ordered{}, err
	}
	after, err := o.After.compile("after")
	if err != nil {
		return Ordered{}, err
	}
	t := readTree(body)
	var out Ordered
	for root := range t.spans {
		if !scope.MatchString(t.lines[root]) {
			continue
		}
		name := t.spanName(root)
		out.Scopes = append(out.Scopes, name)
		b, a := before.pick(t, root), after.pick(t, root)
		if len(b) == 0 || len(a) == 0 {
			out.Missing = append(out.Missing, name)
			continue
		}
		last, first := b[0], a[0]
		for _, i := range b {
			if before.at(t, i) > before.at(t, last) {
				last = i
			}
		}
		for _, i := range a {
			if after.at(t, i) < after.at(t, first) {
				first = i
			}
		}
		if before.at(t, last) > after.at(t, first) {
			out.Bad = append(out.Bad, fmt.Sprintf("%s: %s at %d is after %s at %d", name, t.spanName(last),
				before.at(t, last), t.spanName(first), after.at(t, first)))
			continue
		}
		out.OK = append(out.OK, name)
	}
	return out, nil
}

func (e OrderEnd) compile(side string) (orderEnd, error) {
	if e.At != "start" && e.At != "end" {
		return orderEnd{}, fmt.Errorf("tempo: order.%s.at %q: want start or end", side, e.At)
	}
	match, err := regexp.Compile(e.Match)
	if err != nil {
		return orderEnd{}, fmt.Errorf("tempo: order.%s.match %q: %w", side, e.Match, err)
	}
	parent, err := compile("order."+side+".parent", e.Parent)
	if err != nil {
		return orderEnd{}, err
	}
	grandparent, err := compile("order."+side+".grandparent", e.Grandparent)
	if err != nil {
		return orderEnd{}, err
	}
	return orderEnd{match: match, parent: parent, grandparent: grandparent, end: e.At == "end", first: e.First}, nil
}

// pick is the spans of the subtree of root this end takes, in start order; with first, the first of them only.
func (e orderEnd) pick(t tree, root int) []int {
	var picked []int
	for i := range t.spans {
		if !t.under(i, root) || !e.match.MatchString(t.lines[i]) {
			continue
		}
		p := t.parent(i)
		if e.parent != nil && !e.parent.MatchString(t.lineOf(p)) ||
			e.grandparent != nil && !e.grandparent.MatchString(t.lineOf(t.parent(p))) {
			continue
		}
		picked = append(picked, i)
		if e.first {
			break
		}
	}
	return picked
}

// at is the start or the end of span i in unix nanos.
func (e orderEnd) at(t tree, i int) int64 {
	if e.end {
		return nanos(t.spans[i].End)
	}
	return nanos(t.spans[i].Start)
}

// spanName is "<service> · <span> · <span id>" of span i.
func (t tree) spanName(i int) string {
	s := t.spans[i]
	return s.service + " · " + s.Name + " · " + s.SpanID
}
