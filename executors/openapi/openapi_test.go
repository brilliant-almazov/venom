package openapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

const doc = `{
 "paths": {
  "/v1/billing/accounts/{account_id}": {
   "parameters": [{"name": "account_id", "in": "path", "required": true, "schema": {"type": "string", "example": "1234567890123456789"}}],
   "delete": {"x-required-role": "ROLE_ROOT", "responses": {"200": {}, "401": {}, "403": {}, "422": {}}},
   "query": {"x-required-role": "ROLE_ROOT", "x-auth-roles": ["ROLE_ROOT", "ROLE_USER"], "responses": {"200": {}, "403": {}}}
  },
  "/v1/billing/accounts": {
   "post": {
    "x-required-role": "ROLE_ROOT",
    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Create"}}}},
    "responses": {"200": {}, "403": {}}
   },
   "get": {
    "x-required-role": "ROLE_USER",
    "parameters": [
     {"name": "page_size", "in": "query", "required": true, "schema": {"type": "integer", "minimum": 1}},
     {"name": "status", "in": "query", "schema": {"type": "string", "enum": ["STATUS_UNSPECIFIED", "STATUS_ACTIVE"]}},
     {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 5, "example": 50}}
    ],
    "responses": {"200": {}}
   }
  },
  "/v1/billing/plans/{plan_id}": {
   "put": {"x-required-role": "ROLE_ROOT", "parameters": [{"name": "plan_id", "in": "path", "required": true}], "responses": {"200": {}}}
  }
 },
 "components": {"schemas": {
  "Create": {"allOf": [{"$ref": "#/components/schemas/Owner"}], "required": ["definition_id", "kind", "limits", "note"], "properties": {
   "definition_id": {"type": "string", "example": "7"},
   "kind": {"type": "string", "enum": ["KIND_UNSPECIFIED", "KIND_MAIN"]},
   "limits": {"type": "object", "required": ["cap"], "properties": {"cap": {"type": "integer", "minimum": 0}}},
   "note": {"type": "string"},
   "comment": {"type": "string", "example": "by qa"}
  }},
  "Owner": {"required": ["tenant_id"], "properties": {"tenant_id": {"type": "string", "example": "9007199254740993"}}}
 }}
}`

func describe(method, path, role string) Result {
	return describePlan(method, path, role, Plan{})
}

func describePlan(method, path, role string, plan Plan) Result {
	var r Result
	d := map[string]interface{}{}
	dec := decoder(doc)
	_ = dec.Decode(&d)
	r.Found, r.Allowed = "no", "no"
	Describe(d, method, path, role, plan, &r)
	return r
}

func TestDescribeRoleNotAllowed(t *testing.T) {
	r := describe("DELETE", "/v1/billing/accounts/{account_id}", "ROLE_USER")
	assert.Equal(t, "yes", r.Found)
	assert.Equal(t, "ROLE_ROOT", r.RequiredRole)
	assert.Equal(t, "200,401,403,422", r.Responses)
	assert.Equal(t, "no", r.Allowed)
	assert.Equal(t, "403", r.Expected)
	assert.Equal(t, "/v1/billing/accounts/1234567890123456789", r.URLPath, "the path parameter of the path item, digits kept")
}

func TestDescribeAuthRolesWin(t *testing.T) {
	r := describe("QUERY", "/v1/billing/accounts/{account_id}", "ROLE_USER")
	assert.Equal(t, "ROLE_ROOT,ROLE_USER", r.AuthRoles)
	assert.Equal(t, "yes", r.Allowed)
	assert.Equal(t, "200", r.Expected)
}

func TestDescribeRequiredRoleEqual(t *testing.T) {
	r := describe("GET", "/v1/billing/accounts", "ROLE_USER")
	assert.Equal(t, "yes", r.Allowed)
	assert.Equal(t, "200", r.Expected)
	assert.Equal(t, "page_size=1", r.Query, "only required query parameters, by minimum")
}

func TestDescribeBody(t *testing.T) {
	r := describe("POST", "/v1/billing/accounts", "ROLE_USER")
	assert.Equal(t, "403", r.Expected)
	assert.JSONEq(t, `{"definition_id":"7","kind":"KIND_MAIN","limits":{"cap":0},"tenant_id":"9007199254740993"}`, r.Body,
		"allOf merged, enum without UNSPECIFIED, a nested required object, a field without a value left out")
}

func TestDescribeNoDeclaredCode(t *testing.T) {
	r := describe("PUT", "/v1/billing/plans/{plan_id}", "ROLE_USER")
	assert.Equal(t, "", r.Expected, "403 is not declared: no expected code")
	assert.Equal(t, "", r.URLPath, "a path parameter without a value leaves no path")
}

func TestDescribeMissingOperation(t *testing.T) {
	r := describe("PATCH", "/v1/billing/accounts", "ROLE_USER")
	assert.Equal(t, "no", r.Found)
}

func TestRunReadsTheDocument(t *testing.T) {
	var parent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parent = r.Header.Get("traceparent")
		assert.Equal(t, "Mozilla/5.0", r.Header.Get("User-Agent"))
		w.Header().Set("X-Trace-Id", "0af7651916cd43dd8448eb211c80319c")
		_, _ = w.Write([]byte(doc))
	}))
	defer srv.Close()
	out, err := Executor{}.Run(context.Background(), venom.TestStep{
		"url": srv.URL, "method": "delete", "path": "/v1/billing/accounts/{account_id}", "role": "ROLE_USER",
		"headers": map[string]interface{}{"User-Agent": "Mozilla/5.0"}, "trace": "auto",
	})
	require.NoError(t, err)
	r := out.(Result)
	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", r.TraceID, "the trace id the gateway answers")
	assert.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`, parent)
	assert.Equal(t, "403", r.Expected)
}

func TestRunNotJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>"))
	}))
	defer srv.Close()
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"url": srv.URL, "method": "get", "path": "/x"})
	require.NoError(t, err)
	assert.Equal(t, "no", out.(Result).Found)
}

func TestDescribePlanBody(t *testing.T) {
	r := describePlan("POST", "/v1/billing/accounts", "ROLE_ROOT", Plan{
		Fill:   []string{"comment"},
		Omit:   []string{"kind"},
		Static: map[string]interface{}{"tenant_id": "42"},
		Rows:   `{"data":{"items":[{"Note":"from the list","definitionId":"9"}]}}`,
	})
	assert.JSONEq(t, `{"definition_id":"7","limits":{"cap":0},"note":"from the list","comment":"by qa","tenant_id":"42"}`, r.Body,
		"fill adds comment, omit drops kind, static sets tenant_id, the row gives note that has no example or enum; definition_id keeps its example")
}

func TestDescribePlanQuery(t *testing.T) {
	r := describePlan("GET", "/v1/billing/accounts", "ROLE_USER", Plan{QueryFields: []string{"limit", "status"}})
	assert.Equal(t, "limit=5&page_size=1&status=STATUS_ACTIVE", r.Query, "a query_fields param by its minimum, then its value")
	r = describePlan("GET", "/v1/billing/accounts", "ROLE_USER", Plan{QueryFields: []string{"status"}, Omit: []string{"page_size"}})
	assert.Equal(t, "status=STATUS_ACTIVE", r.Query, "an omitted param is left out even when required")
}

func TestFirstRow(t *testing.T) {
	assert.Equal(t, map[string]interface{}{"accountid": "1"}, firstRow(`{"data":[{"account_id":"1"}]}`))
	assert.Equal(t, map[string]interface{}{"accountid": "2"}, firstRow(`{"data":{"account_id":"2"}}`))
	assert.Nil(t, firstRow(""))
}
