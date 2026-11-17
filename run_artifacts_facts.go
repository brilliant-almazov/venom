package venom

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// StepFacts are what a step calls, as the zoom schemes of a run folder draw it: the method and full url of an http or
// grpc call, the database and the SQL text verbatim of an sql step, the assertions as the scenario writes them.
type StepFacts struct {
	Method, URL string
	// Database is the driver and the database name of an sql step, never its credentials.
	Database   string
	SQL        []string
	Assertions []string
}

// stepFactsOf are the facts of a step from its fields m, the computed request of r when it ran (r nil when it did not).
func stepFactsOf(m map[string]interface{}, r *TestStepResult) StepFacts {
	str := func(k string) string { return protocolField(m[k]) }
	f := StepFacts{Method: str("method"), URL: str("url")}

	if r != nil {
		if method, ok := protocolHTTPMethod(r); ok {
			f.Method, f.URL = fmt.Sprint(method), fmt.Sprint(r.ComputedVars["result.request.url"])
		}
	}

	if s := str("service"); s != "" {
		f.Method = s + "/" + str("method")
	}

	if p := str("path"); p != "" && f.URL != "" && !strings.Contains(f.URL, p) {
		f.URL = strings.TrimRight(f.URL, "/") + "/" + strings.TrimLeft(p, "/")
	}

	if dsn := str("dsn"); dsn != "" || str("driver") != "" {
		f.Database = strings.TrimSpace(str("driver") + " " + dsnDatabase(dsn))
		f.URL = ""
	}

	if list, ok := m["commands"].([]interface{}); ok {
		for _, c := range list {
			f.SQL = append(f.SQL, strings.TrimSpace(fmt.Sprint(c)))
		}
	}

	if file := str("file"); file != "" && f.Database != "" {
		f.SQL = append(f.SQL, "file: "+file)
	}

	if list, ok := m["assertions"].([]interface{}); ok {
		for _, a := range list {
			f.Assertions = append(f.Assertions, strings.TrimSpace(fmt.Sprint(a)))
		}
	}

	return f
}

// dsnKeyValue is a database of a key=value DSN: dbname=main host=db.
var dsnKeyValue = regexp.MustCompile(`(?:^|\s)dbname=(\S+)`)

// dsnDatabase is the database name of a DSN — postgres://u:p@host/main, dbname=main, u:p@tcp(host)/main — with no
// credentials, empty when the DSN names none.
func dsnDatabase(dsn string) string {
	if m := dsnKeyValue.FindStringSubmatch(dsn); m != nil {
		return m[1]
	}

	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" && u.Host != "" {
		return strings.TrimPrefix(u.Path, "/")
	}

	if i := strings.LastIndex(dsn, "/"); i >= 0 {
		return strings.SplitN(dsn[i+1:], "?", 2)[0]
	}

	return ""
}
