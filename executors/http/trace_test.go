package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/ovh/venom"
)

func TestTraceAutoSendsTraceparentAndExportsTheTraceID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	out, err := Executor{}.Run(context.Background(), venom.TestStep{"method": "POST", "url": srv.URL, "body": "{}", "trace": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(r.TraceID) {
		t.Fatalf("trace_id %q", r.TraceID)
	}
	if !regexp.MustCompile(`^00-` + r.TraceID + `-[0-9a-f]{16}-01$`).MatchString(got) {
		t.Errorf("traceparent %q for trace %s", got, r.TraceID)
	}
	if r.TraceSaved != "no" {
		t.Errorf("trace_saved without tempo %q", r.TraceSaved)
	}

	given := "0123456789abcdef0123456789abcdef"
	out, _ = Executor{}.Run(context.Background(), venom.TestStep{"url": srv.URL, "trace": given})
	if out.(Result).TraceID != given {
		t.Errorf("given trace id not kept: %q", out.(Result).TraceID)
	}
	out, _ = Executor{}.Run(context.Background(), venom.TestStep{"url": srv.URL})
	if out.(Result).TraceID != "" {
		t.Errorf("trace id without trace: %q", out.(Result).TraceID)
	}
}
