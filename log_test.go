package venom

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestHideSensitive(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ContextKey("secrets"), []string{"Joe", "Doe"})
	assert.Equal(t, SecretMark, HideSensitive(ctx, "Joe"))
	assert.Equal(t, SecretMark+" tests something", HideSensitive(ctx, "Joe tests something"))
	assert.Equal(t, "Dave tests something", HideSensitive(ctx, "Dave tests something"))
	assert.Equal(t, "1234", HideSensitive(ctx, 1234))
	assert.Equal(t, SecretMark+"!", HideSensitive(ctx, "Doe!"))
	assert.Equal(t, SecretMark+" "+SecretMark, HideSensitive(ctx, "Joe Doe"))
}

func TestLogFunctionsRedactSecrets(t *testing.T) {
	InitTestLogger(t)
	ctx := context.WithValue(context.Background(), ContextKey("secrets"), []string{"my_secret"})

	var buf bytes.Buffer
	logger.Logger.SetOutput(&buf)
	logger.Logger.SetLevel(logrus.DebugLevel)
	defer logger.Logger.SetOutput(os.Stdout)

	Info(ctx, "step content: basic_auth_password: %s", "my_secret")
	assert.NotContains(t, buf.String(), "my_secret")
	assert.Contains(t, buf.String(), SecretMark)

	buf.Reset()
	Debug(ctx, "with vars: %v", H{"basic_auth_password": "my_secret"})
	assert.NotContains(t, buf.String(), "my_secret")
	assert.Contains(t, buf.String(), SecretMark)

	buf.Reset()
	Error(ctx, "interpolated step:\n%s", "basic_auth_password: my_secret\n")
	assert.NotContains(t, buf.String(), "my_secret")
	assert.Contains(t, buf.String(), SecretMark)
}

func TestLogFunctionsPreserveNonStringFormatting(t *testing.T) {
	InitTestLogger(t)
	ctx := context.WithValue(context.Background(), ContextKey("secrets"), []string{"my_secret"})

	var buf bytes.Buffer
	logger.Logger.SetOutput(&buf)
	logger.Logger.SetLevel(logrus.DebugLevel)
	defer logger.Logger.SetOutput(os.Stdout)

	Info(ctx, "Step #%d-%d content is: %s", 3, 7, "my_secret")
	out := buf.String()
	assert.NotContains(t, out, "%!d")
	assert.Contains(t, out, "Step #3-7 content is: "+SecretMark)

	buf.Reset()
	Debug(ctx, "count=%d ratio=%.2f ok=%t", 42, 1.5, true)
	out = buf.String()
	assert.NotContains(t, out, "%!")
	assert.Contains(t, out, "count=42 ratio=1.50 ok=true")
}

// Every credential is replaced by <secret> in the protocols and in raw, declared secret or not: the Supabase Apikey of
// run 17-5 (protocols/SB-06-01-…/setup.md:32 and raw/io.jsonl), an Authorization Bearer, a password, a token. The
// idempotency key is no credential; a text hidden twice stays the same.
func TestCredentialsBecomeSecretMark(t *testing.T) {
	key := "sb_publishable_" + strings.Repeat("a", 31)
	cases := map[string]string{
		"  - Apikey: " + key:        "  - Apikey: " + SecretMark,
		"  - apikey: plainvalue123": "  - apikey: " + SecretMark,
		`{"request_headers":{"Apikey":"plainvalue123","X-Idempotency-Key":"NloBboqV"}}`: `{"request_headers":{"Apikey":"` + SecretMark + `","X-Idempotency-Key":"NloBboqV"}}`,
		`"result.request.header.Authorization":"Bearer abc.def"`:                        `"result.request.header.Authorization":"Bearer ` + SecretMark + `"`,
		"Authorization: Basic dXNlcjpwYXNz":                                             "Authorization: Basic " + SecretMark,
		`{"password":"hunter2","access_token":"tok-1","refresh_token":"r"}`:             `{"password":"` + SecretMark + `","access_token":"` + SecretMark + `","refresh_token":"` + SecretMark + `"}`,
		"body " + key + " end":                              "body " + SecretMark + " end",
		"map[Apikey:plainvalue123 Accept:json]":             "map[Apikey:" + SecretMark + " Accept:json]",
		"  - X-Idempotency-Key: NloBboqV":                   "  - X-Idempotency-Key: NloBboqV",
		`{"password":null,"pin_secret":12,"Secrets":["a"]}`: `{"password":null,"pin_secret":"` + SecretMark + `","Secrets":["` + SecretMark + `"]}`,
	}
	for in, want := range cases {
		got := HideSensitive(context.Background(), in)
		assert.Equal(t, want, got, in)
		assert.Equal(t, want, HideSensitive(context.Background(), got), "hidden twice: "+in)
	}
}

// A header printed by Go as a map of lists, map[Authorization:[Bearer …]], keeps its scheme and hides its token.
func TestCredentialInAGoMapOfLists(t *testing.T) {
	got := HideSensitive(context.Background(), "header:map[Authorization:[Bearer secret-token] X-Idempotency-Key:[NloBboqV]]")
	assert.Equal(t, "header:map[Authorization:[Bearer "+SecretMark+"] X-Idempotency-Key:[NloBboqV]]", got)
}
