package grpc

import (
	"testing"

	"github.com/ovh/venom/executors/hostsuffix"
)

func TestWithHostSuffix(t *testing.T) {
	old := hostsuffix.Lookup
	defer func() { hostsuffix.Lookup = old }()
	hostsuffix.Lookup = func(string) bool { return true }
	cases := map[[2]string]string{
		{"svc-tool-server:50051", "cgw-crawler-chain.orb.local"}: "svc-tool-server.cgw-crawler-chain.orb.local:50051",
		{"svc-tool-server:50051", ""}:                            "svc-tool-server:50051",
		{"a.b:1", "x.local"}:                                     "a.b:1",
		{"localhost:1", "x.local"}:                               "localhost:1",
	}
	for in, want := range cases {
		if got := withHostSuffix(in[0], in[1]); got != want {
			t.Errorf("%v: got %s want %s", in, got, want)
		}
	}
}
