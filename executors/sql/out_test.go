package sql

import (
	"testing"
	"time"
)

func TestCellPrintsLikePsql(t *testing.T) {
	ts := time.Date(2026, 11, 11, 10, 44, 11, 197920000, time.UTC)
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""}, {true, "t"}, {false, "f"}, {int64(12), "12"}, {"a|b", "a|b"}, {ts, "2026-11-11 10:44:11.19792"},
	}
	for _, c := range cases {
		if got := cell(c.in); got != c.want {
			t.Errorf("cell(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
