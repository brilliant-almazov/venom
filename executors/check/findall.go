package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ovh/venom"
)

// FindAll is every match of re over value, or its first capture group when re has one, each replaced by replace
// ([<regexp>, <replacement>]) when given.
func FindAll(value, re string, replace []string) ([]string, error) {
	find, err := regexp.Compile(re)
	if err != nil {
		return nil, fmt.Errorf("check: find_all %q: %w", re, err)
	}
	var from *regexp.Regexp
	to := ""
	switch len(replace) {
	case 0:
	case 2:
		if from, err = regexp.Compile(replace[0]); err != nil {
			return nil, fmt.Errorf("check: replace %q: %w", replace[0], err)
		}
		to = replace[1]
	default:
		return nil, fmt.Errorf("check: replace takes [<regexp>, <replacement>], got %d items", len(replace))
	}
	matches := []string{}
	for _, m := range find.FindAllStringSubmatch(value, -1) {
		s := m[0]
		if len(m) > 1 {
			s = m[1]
		}
		if from != nil {
			s = from.ReplaceAllString(s, to)
		}
		matches = append(matches, s)
	}
	return matches, nil
}

// Duplicates is the matches that occur more than once, each once, in the order of their first occurrence.
func Duplicates(matches []string) []string {
	seen := map[string]int{}
	out := []string{}
	for _, m := range matches {
		seen[m]++
		if seen[m] == 2 {
			out = append(out, m)
		}
	}
	return out
}

func (e Executor) findAll(r *Result) error {
	matches, err := FindAll(e.Value, e.FindAll, e.Replace)
	if err != nil {
		return err
	}
	r.Count = len(matches)
	r.Matches = jsonList(matches)
	r.MatchesJSON = jsonItems(matches)
	if e.Duplicates {
		r.Duplicates = jsonList(Duplicates(matches))
	}
	if e.Join != "" {
		r.Joined = strings.Join(matches, e.Join)
	}
	return nil
}

// jsonList is the items as a JSON array: an item that is a JSON object or array is put as it is, any other as a string.
func jsonList(items []string) string {
	b, _ := json.Marshal(jsonItems(items))
	return string(b)
}

func jsonItems(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		t := strings.TrimSpace(s)
		var v interface{}
		if (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && venom.JSONUnmarshal([]byte(t), &v) == nil {
			out = append(out, v)
			continue
		}
		out = append(out, s)
	}
	return out
}
