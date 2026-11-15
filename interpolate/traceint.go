package interpolate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var traceHex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// traceint is the first 12 hex digits of a 32-hex W3C trace id as an int64, at most 2^48-1: the n of hashids and the
// operation_id of a write, keyed on the trace of the call just before it (cgw-architecture decision 02b). An empty or
// non-hex trace id is an error, never 0.
func traceint(id interface{}) (int64, error) {
	s := strings.ToLower(strings.TrimSpace(strval(id)))
	if !traceHex32.MatchString(s) {
		return 0, fmt.Errorf("traceint: %q is not a 32-hex trace id", s)
	}
	return strconv.ParseInt(s[:12], 16, 64)
}
