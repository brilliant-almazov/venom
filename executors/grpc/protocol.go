package grpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ovh/venom/executors/hostsuffix"
)

var protocolMu sync.Mutex

// withHostSuffix appends suffix to a bare host when the suffixed name resolves (executors/hostsuffix).
func withHostSuffix(url, suffix string) string {
	return hostsuffix.HostPort(url, suffix)
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
