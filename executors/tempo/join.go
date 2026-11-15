package tempo

import (
	"fmt"
	"regexp"
)

// JoinSide picks values from a trace: of each span whose line (SpanLines) matches Match, and whose parent's line
// matches Parent when Parent is given, the value of the attribute whose key is Attr, and of that value the one group
// of Extract when Extract is given.
type JoinSide struct {
	Match   string `json:"match" yaml:"match" mapstructure:"match"`
	Parent  string `json:"parent" yaml:"parent" mapstructure:"parent"`
	Attr    string `json:"attr" yaml:"attr" mapstructure:"attr"`
	Extract string `json:"extract" yaml:"extract" mapstructure:"extract"`
}

// Join pairs the values of Left with the values of Right over one saved trace, with no TraceQL.
type Join struct {
	Left  JoinSide `json:"left" yaml:"left" mapstructure:"left"`
	Right JoinSide `json:"right" yaml:"right" mapstructure:"right"`
}

// Joined is a join over one trace: Left and Right in span order, Pairs and Missing each value once in Left order.
type Joined struct {
	Left, Right, Pairs, Missing []string
}

// Of joins over one trace: Pairs are the left values with an equal right value, Missing the left values with none.
func (j Join) Of(body []byte) (Joined, error) {
	left, err := j.Left.values(body, "left")
	if err != nil {
		return Joined{}, err
	}
	right, err := j.Right.values(body, "right")
	if err != nil {
		return Joined{}, err
	}
	out := Joined{Left: left, Right: right}
	onRight := map[string]bool{}
	for _, v := range right {
		onRight[v] = true
	}
	seen := map[string]bool{}
	for _, v := range left {
		if seen[v] {
			continue
		}
		seen[v] = true
		if onRight[v] {
			out.Pairs = append(out.Pairs, v)
		} else {
			out.Missing = append(out.Missing, v)
		}
	}
	return out, nil
}

// values is the value of attribute Attr of each span whose line matches Match (and whose parent's line matches
// Parent), in span start order; with Extract, its one group, and a value Extract does not match is left out.
func (s JoinSide) values(body []byte, side string) ([]string, error) {
	match, err := regexp.Compile(s.Match)
	if err != nil {
		return nil, fmt.Errorf("tempo: join.%s.match %q: %w", side, s.Match, err)
	}
	parent, err := compile("join."+side+".parent", s.Parent)
	if err != nil {
		return nil, err
	}
	var extract *regexp.Regexp
	if s.Extract != "" {
		if extract, err = regexp.Compile(s.Extract); err != nil {
			return nil, fmt.Errorf("tempo: join.%s.extract %q: %w", side, s.Extract, err)
		}
		if extract.NumSubexp() != 1 {
			return nil, fmt.Errorf("tempo: join.%s.extract %q: want one group, got %d", side, s.Extract, extract.NumSubexp())
		}
	}
	t := readTree(body)
	var values []string
	for i, sp := range t.spans {
		if !match.MatchString(t.lines[i]) || parent != nil && !parent.MatchString(t.lineOf(t.parent(i))) {
			continue
		}
		for _, a := range sortedAttrs(sp) {
			if a.Key != s.Attr {
				continue
			}
			v := a.text()
			if extract != nil {
				m := extract.FindStringSubmatch(v)
				if m == nil {
					continue
				}
				v = m[1]
			}
			values = append(values, v)
		}
	}
	return values, nil
}
