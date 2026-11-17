package venom

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func InitTestLogger(t *testing.T) {
	l := logrus.New()
	logger = logrus.NewEntry(l)
}

var (
	logger *logrus.Entry
	fields = []string{"testsuite", "testcase", "step", "executor"}
)

func fieldsFromContext(ctx context.Context, keys ...string) logrus.Fields {
	fields := logrus.Fields{}
	if ctx == nil {
		return fields
	}
	for _, k := range keys {
		ck := ContextKey(k)
		i := ctx.Value(ck)
		if i != nil {
			fields[k] = i
		}
	}
	return fields
}

func asJsonString(i interface{}) string {
	btes, _ := json.Marshal(i)
	return string(btes)
}

// SecretMark replaces every secret in every output of a run: the protocols, the span trees, io.jsonl, venom.log, the
// reports and the dumps.
const SecretMark = "<secret>"

// HideSensitive replace the value with SecretMark: every declared secret, then every credential (maskCredentials).
func HideSensitive(ctx context.Context, arg interface{}) string {
	s, ok := arg.(string)
	if !ok {
		s = fmt.Sprint(arg)
	}
	if secrets, ok := ctx.Value(ContextKey("secrets")).([]string); ok && len(secrets) > 0 {
		s = replaceSecrets(s, secrets)
	}
	return maskCredentials(s)
}

// credentialURL is a URL with a password in its userinfo, scheme://user:password@host; credentialJWT is a JWT,
// header.payload.signature in base64url with the header starting {" (eyJ); credentialKey is a Supabase API key
// (sb_publishable_…, sb_secret_…); credentialNamed is the value of a header, field or var named as a credential —
// Apikey, X-Api-Key, Authorization (its scheme Bearer, Basic or Token kept), Cookie, Set-Cookie, and every name holding
// token, secret, password or passwd — written `name: value`, `name=value` or `"name":"value"`. X-Idempotency-Key is no
// credential and stays as it is.
var (
	credentialURL    = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s:/@"'<>]+:)[^\s@/"'<>]+@`)
	credentialJWT    = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	credentialKey    = regexp.MustCompile(`\bsb_(?:publishable|secret)_[A-Za-z0-9_-]+`)
	credentialNamed  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])["']?(?:apikey|api[-_]key|x-api-key|authorization|proxy-authorization|cookie|set-cookie|[a-z0-9_-]*(?:token|secret|password|passwd)[a-z0-9_-]*)["']?[ \t]*[:=][ \t]*\[?["']?`)
	credentialScheme = regexp.MustCompile(`(?i)^(?:bearer|basic|token)[ \t]+`)
)

// maskCredentials hides a credential no testsuite declares as a secret: the password of a URL with userinfo (a
// database DSN, postgresql://postgres:<password>@host), a JWT (a gateway token), a Supabase API key and the value of
// every header, field or var named as a credential. Run 16-4 of cgw-qa-scenarios carried the DSN and the tokens in
// test_results_*.json, 52 times in one file; run 17-5 carried the Supabase Apikey in its protocols and io.jsonl.
func maskCredentials(s string) string {
	s = credentialURL.ReplaceAllString(s, "${1}"+SecretMark+"@")
	s = credentialJWT.ReplaceAllString(s, SecretMark)
	s = credentialKey.ReplaceAllString(s, SecretMark)
	return maskNamedCredentials(s)
}

// maskNamedCredentials replaces the value after every credential name and its separator, the scheme of an
// Authorization kept; a value already hidden stays as it is, so the text can be hidden twice.
func maskNamedCredentials(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range credentialNamed.FindAllStringIndex(s, -1) {
		if m[1] < last {
			continue
		}
		at := m[1]
		if sc := credentialScheme.FindString(s[at:]); sc != "" {
			at += len(sc)
		}
		end := at
		for end < len(s) && !strings.ContainsRune(credentialValueEnd, rune(s[end])) {
			end++
		}
		value := s[at:end]
		if value == "" || strings.HasPrefix(value, SecretMark) || credentialLiteral[value] {
			continue
		}
		b.WriteString(s[last:at])
		if quoted := s[m[0]:m[1]]; strings.Contains(quoted, `"`) && !strings.HasSuffix(quoted, `"`) && at == m[1] {
			b.WriteString(`"` + SecretMark + `"`)
		} else {
			b.WriteString(SecretMark)
		}
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// credentialLiteral are the JSON values after a credential name that hold no credential: JSON stays JSON.
var credentialLiteral = map[string]bool{"null": true, "true": true, "false": true, "{": true}

// credentialValueEnd are the characters a credential value ends at.
const credentialValueEnd = " \t\r\n\"',;}]"

func replaceSecrets(s string, secrets []string) string {
	sorted := append([]string(nil), secrets...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, secret := range sorted {
		if secret == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, SecretMark)
	}
	return s
}

func secretKeySet(secretKeys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(secretKeys))
	for _, k := range secretKeys {
		set[k] = struct{}{}
	}
	return set
}

// redactMapVars hides the declared secret keys and every secret or credential inside a value; a value with nothing
// to hide keeps its type.
func redactMapVars(ctx context.Context, vars H, secretKeys []string) {
	secretSet := secretKeySet(secretKeys)
	for k, val := range vars {
		if _, ok := secretSet[k]; ok && !strings.HasPrefix(k, "venom.") {
			vars[k] = SecretMark
			continue
		}
		if cleaned := HideSensitive(ctx, val); cleaned != fmt.Sprint(val) {
			vars[k] = cleaned
		}
	}
}

func redactStringMap(ctx context.Context, vars map[string]string, secretKeys []string) {
	secretSet := secretKeySet(secretKeys)
	for k, val := range vars {
		if _, ok := secretSet[k]; ok && !strings.HasPrefix(k, "venom.") {
			vars[k] = SecretMark
			continue
		}
		vars[k] = HideSensitive(ctx, val)
	}
}

func redactLogArgs(ctx context.Context, args ...interface{}) []interface{} {
	if ctx == nil {
		return args
	}
	secrets, _ := ctx.Value(ContextKey("secrets")).([]string)
	if len(args) == 0 {
		return args
	}
	redacted := make([]interface{}, len(args))
	for i, arg := range args {
		redacted[i] = redactLogArg(secrets, arg)
	}
	return redacted
}

// redactLogArg redacts a single log argument. It only replaces the argument
// with a string when a secret was actually found, so non-string arguments keep
// their original type and format verbs like %d, %f and %t still work.
func redactLogArg(secrets []string, arg interface{}) interface{} {
	if arg == nil {
		return nil
	}
	original := fmt.Sprint(arg)
	cleaned := maskCredentials(replaceSecrets(original, secrets))
	if cleaned == original {
		return arg
	}
	return cleaned
}

// hideSensitiveBytes redacts secrets in byte-oriented step output while preserving []byte type for JSON encoding.
func hideSensitiveBytes(ctx context.Context, data interface{}) []byte {
	if data == nil {
		return nil
	}
	switch v := data.(type) {
	case []byte:
		return []byte(HideSensitive(ctx, string(v)))
	case string:
		return []byte(HideSensitive(ctx, v))
	default:
		return []byte(HideSensitive(ctx, fmt.Sprint(v)))
	}
}

func appendDerivedSecrets(secrets []string, seen map[string]struct{}, vars H, secretKeys []string) []string {
	secretSet := secretKeySet(secretKeys)
	add := func(value string) {
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}

	for _, key := range secretKeys {
		val := fmt.Sprint(vars[key])
		if val != "" && val != "<nil>" {
			add(base64.StdEncoding.EncodeToString([]byte(val)))
		}
	}

	if _, ok := secretSet["basic_auth_password"]; ok {
		user := fmt.Sprint(vars["basic_auth_user"])
		pass := fmt.Sprint(vars["basic_auth_password"])
		if pass != "" && pass != "<nil>" {
			add(base64.StdEncoding.EncodeToString([]byte(user + ":" + pass)))
		}
	}

	return secrets
}

func Debug(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Debugf(format, redactLogArgs(ctx, args...)...)
}

func Info(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Infof(format, redactLogArgs(ctx, args...)...)
}

func Warn(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Warnf(format, redactLogArgs(ctx, args...)...)
}

func Warning(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Warningf(format, redactLogArgs(ctx, args...)...)
}

func Error(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Errorf(format, redactLogArgs(ctx, args...)...)
}

func Fatal(ctx context.Context, format string, args ...interface{}) {
	fields := fieldsFromContext(ctx, fields...)
	logger.WithFields(fields).Fatalf(format, redactLogArgs(ctx, args...)...)
}
