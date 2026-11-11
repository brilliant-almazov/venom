package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ovh/venom"
)

func TestTruncateBodyGivesTheServerAReadError(t *testing.T) {
	var readErrs int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			atomic.AddInt32(&readErrs, 1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	out, err := Executor{}.Run(context.Background(), venom.TestStep{
		"method":        "POST",
		"url":           srv.URL + "/v2/x?id=1",
		"body":          `{"tasks":[]}`,
		"headers":       map[string]string{"Content-Type": "application/json"},
		"truncate_body": 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(Result)
	if r.StatusCode != http.StatusBadRequest || r.Body != "" {
		t.Errorf("status %d body %q", r.StatusCode, r.Body)
	}
	if atomic.LoadInt32(&readErrs) != 1 {
		t.Errorf("server read errors %d", readErrs)
	}

	out, err = Executor{}.Run(context.Background(), venom.TestStep{
		"method": "POST", "url": srv.URL + "/v2/x", "body": `{"tasks":[]}`,
	})
	if err != nil || out.(Result).StatusCode != http.StatusAccepted {
		t.Errorf("whole body: %v %+v", err, out)
	}
}
