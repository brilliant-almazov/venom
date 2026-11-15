// Package hostsuffix appends a DNS suffix to a bare host (a compose service name) when the suffixed name resolves.
//
// On the machine, OrbStack resolves <service>.<compose project>.orb.local; inside a container on the stand network the bare
// service name resolves and the suffixed one does not, so the same step works in both places.
package hostsuffix

import (
	"net"
	"net/url"
	"strings"
)

// Lookup resolves a host; tests replace it.
var Lookup = func(host string) bool {
	_, err := net.LookupHost(host)
	return err == nil
}

// Host returns host+"."+suffix when host has no dot, is not localhost and the suffixed name resolves; else host.
func Host(host, suffix string) string {
	suffix = strings.TrimPrefix(suffix, ".")
	if suffix == "" || host == "" || strings.Contains(host, ".") || host == "localhost" || net.ParseIP(host) != nil {
		return host
	}
	if full := host + "." + suffix; Lookup(full) {
		return full
	}
	return host
}

// HostPort applies Host to the host of "host:port".
func HostPort(hostport, suffix string) string {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return net.JoinHostPort(Host(host, suffix), port)
}

// URL applies Host to the host of a URL (http://s3:9000, postgres://u:p@postgres:5432/db?x=y).
func URL(raw, suffix string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host, port := u.Hostname(), u.Port()
	host = Host(host, suffix)
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	return u.String()
}
