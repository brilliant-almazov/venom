package hostsuffix

import "testing"

func TestHostOnlyWhenTheSuffixedNameResolves(t *testing.T) {
	old := Lookup
	defer func() { Lookup = old }()
	Lookup = func(h string) bool { return h == "svc.p.orb.local" || h == "postgres.p.orb.local" }
	cases := map[[2]string]string{
		{"svc:50051", "p.orb.local"}:   "svc.p.orb.local:50051",
		{"other:50051", "p.orb.local"}: "other:50051",
		{"a.b:1", "p.orb.local"}:       "a.b:1",
		{"svc:50051", ""}:              "svc:50051",
	}
	for in, want := range cases {
		if got := HostPort(in[0], in[1]); got != want {
			t.Errorf("%v: %s want %s", in, got, want)
		}
	}
	if got := URL("postgres://u:p@postgres:5432/db?sslmode=disable", "p.orb.local"); got != "postgres://u:p@postgres.p.orb.local:5432/db?sslmode=disable" {
		t.Errorf("url %s", got)
	}
	if got := URL("http://10.0.0.1:9000", "p.orb.local"); got != "http://10.0.0.1:9000" {
		t.Errorf("ip %s", got)
	}
}
