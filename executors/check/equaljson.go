package check

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ovh/venom"
)

// EqualJSON says whether two JSON texts hold the same value, keys in any order; with sortLists every list is compared
// as its items in any order. The diff names both values in canonical form when they differ.
func EqualJSON(a, b string, sortLists bool) (bool, string, error) {
	ca, err := canonical(a, sortLists)
	if err != nil {
		return false, "", fmt.Errorf("check: value is not JSON: %w", err)
	}
	cb, err := canonical(b, sortLists)
	if err != nil {
		return false, "", fmt.Errorf("check: equal_json is not JSON: %w", err)
	}
	if ca == cb {
		return true, "", nil
	}
	return false, "value " + ca + "\nequal_json " + cb, nil
}

// canonical is the JSON text with keys sorted (encoding/json sorts map keys) and, with sortLists, every list sorted by
// the canonical text of its items.
func canonical(text string, sortLists bool) (string, error) {
	var v interface{}
	if err := venom.JSONUnmarshal([]byte(text), &v); err != nil {
		return "", err
	}
	b, err := json.Marshal(normalize(v, sortLists))
	return string(b), err
}

func normalize(v interface{}, sortLists bool) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, item := range t {
			t[k] = normalize(item, sortLists)
		}
		return t
	case []interface{}:
		for i, item := range t {
			t[i] = normalize(item, sortLists)
		}
		if !sortLists {
			return t
		}
		keys := make([]string, len(t))
		for i, item := range t {
			b, _ := json.Marshal(item)
			keys[i] = string(b)
		}
		sort.Strings(keys)
		out := make([]json.RawMessage, len(keys))
		for i, k := range keys {
			out[i] = json.RawMessage(k)
		}
		return out
	}
	return v
}
