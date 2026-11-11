package grpc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var protocolMu sync.Mutex

// withHostSuffix appends suffix to a host without a dot: "svc:50051" + "p.orb.local" -> "svc.p.orb.local:50051".
func withHostSuffix(url, suffix string) string {
	if suffix == "" {
		return url
	}
	host, port, err := net.SplitHostPort(url)
	if err != nil || strings.Contains(host, ".") || host == "localhost" {
		return url
	}
	return net.JoinHostPort(host+"."+strings.TrimPrefix(suffix, "."), port)
}

func rawOrText(b []byte) interface{} {
	var v interface{}
	if json.Unmarshal(b, &v) == nil {
		return v
	}
	s := string(b)
	if len(s) > 4096 {
		s = s[:4096]
	}
	return s
}

// writeProtocol appends one JSON line for the call to e.ProtocolFile.
func writeProtocol(e Executor, request []byte, r *Result, callErr error) {
	msg := r.Message
	if callErr != nil && msg == "" {
		msg = callErr.Error()
	}
	line := map[string]interface{}{
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"case":        e.Case,
		"trace_id":    r.TraceID,
		"trace_file":  r.TraceFile,
		"trace_saved": r.TraceSaved,
		"call": map[string]interface{}{
			"kind": "grpc", "method": e.Service + "/" + e.Method, "addr": e.URL, "request": rawOrText(request),
			"code": r.Status, "message": msg, "response": rawOrText([]byte(r.Systemout)),
		},
	}
	b, err := json.Marshal(line)
	if err != nil {
		return
	}
	protocolMu.Lock()
	defer protocolMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(e.ProtocolFile), 0o755)
	f, err := os.OpenFile(e.ProtocolFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}
