// Package openapi reads one operation of a live OpenAPI document and exposes that as the `openapi` executor.
//
// The step GETs the document at `url` (headers as given, a W3C traceparent when `trace` is set) and looks up
// paths[<path>][<method>] by the path template exactly as the document writes it. The result carries only what a
// scenario needs from that operation, never the document itself (a 5.7 MB spec kept as a var went into the computed
// vars of every later step, cgw-qa run 15-4):
//
//   - required_role and auth_roles: the x-required-role and x-auth-roles extensions of the operation;
//   - responses: the declared response codes, sorted, comma separated;
//   - allowed and expected for the caller `role`: allowed when x-auth-roles lists the role, or, without x-auth-roles,
//     when x-required-role equals it; expected is then the lowest declared 2xx code, and 403 when the role is not
//     allowed and the operation declares 403; empty when the document gives neither;
//   - url_path, query and body: the inputs of the call, each value the example of the parameter or field, then its
//     first enum value that is not *_UNSPECIFIED, then (page_size and other integers) its minimum; url_path is empty
//     when a path parameter has none of them, a field without any is left out of the body.
//
// The request plan of a step (Plan) widens the inputs: `fill` names the non-required body fields to fill the same way,
// `omit` the fields and query params to leave out, `static` the body fields set as given, `rows` a list answer whose
// first item gives the value of a field or parameter with no example or enum (its name compared without case and
// `_`), and `query_fields` the optional query params to add, each by its minimum, then its value.
package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/tempo"
)

// Name of the executor.
const Name = "openapi"

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor reads one operation: url, method, path, role, headers, trace, timeout (seconds, default 60).
type Executor struct {
	URL     string            `json:"url" yaml:"url" mapstructure:"url"`
	Method  string            `json:"method" yaml:"method" mapstructure:"method"`
	Path    string            `json:"path" yaml:"path" mapstructure:"path"`
	Role    string            `json:"role" yaml:"role" mapstructure:"role"`
	Headers map[string]string `json:"headers" yaml:"headers" mapstructure:"headers"`
	Trace   string            `json:"trace" yaml:"trace" mapstructure:"trace"`
	Timeout int               `json:"timeout" yaml:"timeout" mapstructure:"timeout"`
	Plan    `mapstructure:",squash"`
}

// Plan is the request plan of a step beyond the required inputs.
type Plan struct {
	Fill        []string               `json:"fill" yaml:"fill" mapstructure:"fill"`
	Omit        []string               `json:"omit" yaml:"omit" mapstructure:"omit"`
	Static      map[string]interface{} `json:"static" yaml:"static" mapstructure:"static"`
	Rows        string                 `json:"rows" yaml:"rows" mapstructure:"rows"`
	QueryFields []string               `json:"query_fields" yaml:"query_fields" mapstructure:"query_fields"`
}

// Result of the executor.
type Result struct {
	StatusCode   int    `json:"statuscode" yaml:"statuscode"`
	TraceID      string `json:"trace_id" yaml:"trace_id"`
	Found        string `json:"found" yaml:"found"`
	RequiredRole string `json:"required_role" yaml:"required_role"`
	AuthRoles    string `json:"auth_roles" yaml:"auth_roles"`
	Responses    string `json:"responses" yaml:"responses"`
	Allowed      string `json:"allowed" yaml:"allowed"`
	Expected     string `json:"expected" yaml:"expected"`
	URLPath      string `json:"url_path" yaml:"url_path"`
	Query        string `json:"query" yaml:"query"`
	Body         string `json:"body" yaml:"body"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: the document is read and the operation is in it.
func (Executor) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.statuscode ShouldEqual 200", "result.found ShouldEqual yes"}}
}

// Run reads the document and the operation.
func (Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	timeout := time.Duration(e.Timeout) * time.Second
	if e.Timeout <= 0 {
		timeout = 60 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range e.Headers {
		req.Header.Set(k, v)
	}
	result := Result{Found: "no", Allowed: "no"}
	if e.Trace != "" {
		result.TraceID = tempo.NewTraceID(e.Trace)
		req.Header.Set("traceparent", "00-"+result.TraceID+"-"+tempo.NewSpanID()+"-01")
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("openapi: GET %s: %w", e.URL, err)
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	if id := resp.Header.Get("X-Trace-Id"); id != "" {
		result.TraceID = id
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openapi: read %s: %w", e.URL, err)
	}
	var doc map[string]interface{}
	dec := decoder(string(raw))
	if resp.StatusCode != http.StatusOK || dec.Decode(&doc) != nil {
		return result, nil
	}
	Describe(doc, e.Method, e.Path, e.Role, e.Plan, &result)
	return result, nil
}

// decoder keeps numbers as the document writes them: a Snowflake example does not go through float64.
func decoder(raw string) *json.Decoder {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	return dec
}

// Describe fills the operation fields of result from doc and the request plan.
func Describe(doc map[string]interface{}, method, path, role string, plan Plan, result *Result) {
	s := spec{doc: doc, plan: plan, row: firstRow(plan.Rows)}
	item := s.resolve(obj(obj(doc["paths"])[path]))
	op := obj(item[strings.ToLower(method)])
	if op == nil {
		return
	}
	result.Found = "yes"
	result.RequiredRole = str(op["x-required-role"])
	var roles []string
	for _, r := range list(op["x-auth-roles"]) {
		roles = append(roles, str(r))
	}
	result.AuthRoles = strings.Join(roles, ",")
	var codes []string
	for code := range obj(op["responses"]) {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	result.Responses = strings.Join(codes, ",")
	allowed := result.RequiredRole != "" && result.RequiredRole == role
	if op["x-auth-roles"] != nil {
		allowed = contains(roles, role)
	}
	if allowed {
		result.Allowed = "yes"
		for _, c := range codes {
			if strings.HasPrefix(c, "2") {
				result.Expected = c
				break
			}
		}
	} else if contains(codes, "403") {
		result.Expected = "403"
	}
	params := append(list(item["parameters"]), list(op["parameters"])...)
	result.URLPath = s.urlPath(path, params)
	result.Query = s.query(params)
	if body := s.body(op, params); body != nil {
		if b, err := json.Marshal(body); err == nil {
			result.Body = string(b)
		}
	}
}

type spec struct {
	doc  map[string]interface{}
	plan Plan
	row  map[string]interface{}
}

// fold is a field name compared without case and `_`.
func fold(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// firstRow is the first item of data.items, data or items of a list answer, else its data object, keyed by fold.
func firstRow(raw string) map[string]interface{} {
	var answer interface{}
	if strings.TrimSpace(raw) == "" || decoder(raw).Decode(&answer) != nil {
		return nil
	}
	data := obj(answer)["data"]
	var row map[string]interface{}
	for _, l := range [][]interface{}{list(obj(data)["items"]), list(data), list(obj(answer)["items"]), list(answer)} {
		if len(l) > 0 {
			row = obj(l[0])
			break
		}
	}
	if row == nil {
		row = obj(data)
	}
	out := map[string]interface{}{}
	for k, v := range row {
		out[fold(k)] = v
	}
	return out
}

// resolve follows $ref and merges allOf.
func (s spec) resolve(o map[string]interface{}) map[string]interface{} {
	for depth := 0; o != nil && depth < 30; depth++ {
		ref, ok := o["$ref"].(string)
		if !ok {
			break
		}
		var cur interface{} = s.doc
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			cur = obj(cur)[strings.NewReplacer("~1", "/", "~0", "~").Replace(part)]
		}
		o = obj(cur)
	}
	if all := list(o["allOf"]); len(all) > 0 {
		m := map[string]interface{}{}
		props := map[string]interface{}{}
		var req []interface{}
		for k, v := range o {
			m[k] = v
		}
		for k, v := range obj(o["properties"]) {
			props[k] = v
		}
		req = append(req, list(o["required"])...)
		for _, p := range all {
			p := s.resolve(obj(p))
			for k, v := range obj(p["properties"]) {
				props[k] = v
			}
			req = append(req, list(p["required"])...)
		}
		m["properties"], m["required"] = props, req
		delete(m, "allOf")
		return m
	}
	return o
}

// value of a parameter or a schema field named name: example, then enum, then the same field of the rows, then minimum.
func (s spec) value(name string, o map[string]interface{}) (interface{}, bool) {
	cands := []map[string]interface{}{s.resolve(o)}
	if sch := obj(cands[0]["schema"]); sch != nil {
		cands = append(cands, s.resolve(sch))
	}
	for _, c := range cands {
		for _, k := range []string{"example", "x-example"} {
			if v, ok := c[k]; ok && v != nil {
				return v, true
			}
		}
		switch ex := c["examples"].(type) {
		case []interface{}:
			if len(ex) > 0 {
				return ex[0], true
			}
		case map[string]interface{}:
			keys := sortedKeys(ex)
			if len(keys) > 0 {
				v := ex[keys[0]]
				if m := obj(v); m != nil {
					if vv, ok := m["value"]; ok {
						return vv, true
					}
				}
				return v, true
			}
		}
	}
	for _, c := range cands {
		enum := list(c["enum"])
		if len(enum) == 0 {
			enum = list(s.resolve(obj(c["items"]))["enum"])
		}
		for _, v := range enum {
			if !strings.HasSuffix(strings.ToUpper(fmt.Sprint(v)), "UNSPECIFIED") {
				return v, true
			}
		}
	}
	if v, ok := s.row[fold(name)]; ok && v != nil {
		return v, true
	}
	for _, c := range cands {
		if m, ok := c["minimum"]; ok {
			return m, true
		}
	}
	return nil, false
}

func (s spec) urlPath(path string, params []interface{}) string {
	out := path
	for _, seg := range strings.Split(path, "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		name := strings.Trim(seg, "{}")
		found := false
		for _, p := range params {
			p := s.resolve(obj(p))
			if str(p["in"]) != "path" || str(p["name"]) != name {
				continue
			}
			if v, ok := s.value(name, p); ok && plain(v) != "" {
				out = strings.Replace(out, seg, url.PathEscape(plain(v)), 1)
				found = true
			}
			break
		}
		if !found {
			return ""
		}
	}
	return out
}

// query of the required params and the query_fields, an omitted one left out; a query_fields param takes its minimum
// first.
func (s spec) query(params []interface{}) string {
	q := url.Values{}
	for _, p := range params {
		p := s.resolve(obj(p))
		name := str(p["name"])
		asked := contains(s.plan.QueryFields, name)
		if str(p["in"]) != "query" || (p["required"] != true && !asked) || contains(s.plan.Omit, name) {
			continue
		}
		if m, ok := s.resolve(obj(p["schema"]))["minimum"]; asked && ok {
			q.Set(name, plain(m))
		} else if v, ok := s.value(name, p); ok {
			q.Set(name, plain(v))
		}
	}
	return q.Encode()
}

func (s spec) body(op map[string]interface{}, params []interface{}) map[string]interface{} {
	var sch map[string]interface{}
	if rb := s.resolve(obj(op["requestBody"])); rb != nil {
		content := obj(rb["content"])
		media := obj(content["application/json"])
		if media == nil {
			for _, k := range sortedKeys(content) {
				media = obj(content[k])
				break
			}
		}
		sch = s.resolve(obj(media["schema"]))
	}
	for _, p := range params {
		if p := s.resolve(obj(p)); sch == nil && str(p["in"]) == "body" {
			sch = s.resolve(obj(p["schema"]))
		}
	}
	if sch == nil {
		if len(s.plan.Static) == 0 {
			return nil
		}
		sch = map[string]interface{}{}
	}
	out := s.object(sch, s.plan.Fill, 0)
	for _, name := range s.plan.Omit {
		delete(out, name)
	}
	for k, v := range s.plan.Static {
		out[k] = v
	}
	return out
}

// object of the required fields of sch and the extra ones named, each by its value; a required object without a value
// is built the same way.
func (s spec) object(sch map[string]interface{}, extra []string, depth int) map[string]interface{} {
	out := map[string]interface{}{}
	props := obj(sch["properties"])
	names := list(sch["required"])
	for _, n := range extra {
		names = append(names, n)
	}
	for _, r := range names {
		name := str(r)
		field := s.resolve(obj(props[name]))
		if field == nil {
			continue
		}
		if v, ok := s.value(name, field); ok {
			out[name] = v
		} else if str(field["type"]) == "object" && depth < 10 {
			out[name] = s.object(field, nil, depth+1)
		}
	}
	return out
}

func obj(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func list(v interface{}) []interface{} {
	l, _ := v.([]interface{})
	return l
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

// plain renders a value as it goes into a path or a query; numbers keep their digits as the document writes them.
func plain(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case []interface{}:
		if len(t) > 0 {
			return plain(t[0])
		}
		return ""
	}
	return fmt.Sprint(v)
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
