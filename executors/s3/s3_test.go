package s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ovh/venom"
)

func TestWithHostSuffix(t *testing.T) {
	cases := map[[2]string]string{
		{"http://s3:9000", "cgw-crawler-chain.orb.local"}: "http://s3.cgw-crawler-chain.orb.local:9000",
		{"http://s3:9000", ""}:                            "http://s3:9000",
		{"http://a.b:9000", "x.local"}:                    "http://a.b:9000",
	}
	for in, want := range cases {
		if got, err := WithHostSuffix(in[0], in[1]); err != nil || got != want {
			t.Errorf("%v: got %s %v want %s", in, got, err, want)
		}
	}
}

func run(t *testing.T, url string, step venom.TestStep) Result {
	t.Helper()
	step["endpoint"] = url
	step["bucket"] = "b"
	step["access_key"] = "qa"
	step["secret_key"] = "qa"
	out, err := Executor{}.Run(context.Background(), step)
	if err != nil {
		t.Fatal(err)
	}
	return out.(Result)
}

func TestActions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Has("lifecycle"):
			_, _ = w.Write([]byte(`<LifecycleConfiguration><Rule><ID>expire</ID><Status>Enabled</Status>` +
				`<Filter><Prefix>bodies/</Prefix></Filter><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`))
		case r.URL.Query().Get("list-type") == "2":
			_, _ = w.Write([]byte(`<ListBucketResult><Contents><Key>bodies/1</Key></Contents><Contents><Key>bodies/2</Key></Contents></ListBucketResult>`))
		case strings.HasSuffix(r.URL.Path, "/missing"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodHead:
			w.Header().Set("Content-Length", "5")
			w.Header().Set("Content-Type", "application/json")
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"a":1}`))
		}
	}))
	defer srv.Close()

	if r := run(t, srv.URL, venom.TestStep{"action": "lifecycle"}); r.RulesCount != 1 || r.Rules[0].ExpirationDays != 30 || r.Rules[0].Prefix != "bodies/" {
		t.Fatalf("lifecycle %+v", r)
	}
	if r := run(t, srv.URL, venom.TestStep{"action": "list", "prefix": "bodies/"}); r.Count != 2 || r.Keys[1] != "bodies/2" {
		t.Fatalf("list %+v", r)
	}
	if r := run(t, srv.URL, venom.TestStep{"action": "head", "key": "k"}); !r.Exists || r.Size != 5 {
		t.Fatalf("head %+v", r)
	}
	if r := run(t, srv.URL, venom.TestStep{"action": "head", "key": "missing"}); r.Exists || r.Err != "" {
		t.Fatalf("head missing %+v", r)
	}
	if r := run(t, srv.URL, venom.TestStep{"action": "get", "key": "k"}); r.Content != `{"a":1}` {
		t.Fatalf("get %+v", r)
	}
}
