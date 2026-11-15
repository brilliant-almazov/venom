package grpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const billingProto = `syntax = "proto3";
package billing.v1;
import "google/protobuf/timestamp.proto";
enum Kind { KIND_UNSPECIFIED = 0; KIND_MAIN = 1; KIND_SUB = 2; }
message Money { int64 units = 1; string currency = 2; }
message CreateRateSourceRequest {
  int64 operation_id = 1;
  string name = 2;
  Kind kind = 3;
  bool active = 4;
  Money limit = 5;
  google.protobuf.Timestamp starts_at = 6;
  repeated string tags = 7;
  optional string note = 8;
  oneof target { string account_id = 9; string tenant_id = 10; }
  int32 priority = 11;
}
message CreateRateSourceResponse { string id = 1; }
service RateSourceService { rpc CreateRateSource(CreateRateSourceRequest) returns (CreateRateSourceResponse); }
`

func billingSource(t *testing.T) Executor {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "billing.proto"), []byte(billingProto), 0o644))
	return Executor{ProtoFiles: []string{filepath.Join(dir, "billing.proto")}, ImportPaths: []string{dir}}
}

func TestRequestFromReflection(t *testing.T) {
	e := billingSource(t)
	src, err := e.fileSource("")
	require.NoError(t, err)
	raw, err := requestFromReflection(src, "billing.v1.RateSourceService", "CreateRateSource",
		map[string]string{"operation_id": "12057669342925", "limit.currency": "EUR", "tags": `["a"]`, "account_id": "7", "priority": "3"},
		[]byte(`{"name":"from data","limit":{"units":"5"}}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "operation_id": "12057669342925",
	  "name": "from data",
	  "kind": "KIND_MAIN",
	  "active": false,
	  "limit": {"units": "5", "currency": "EUR"},
	  "starts_at": "2026-01-01T00:00:00Z",
	  "tags": ["a"],
	  "account_id": "7",
	  "priority": 3
	}`, string(raw), "singular fields by default, fill by path converted to the type, data merged over; optional and repeated only by fill")
}

func TestBuildRequestDefaults(t *testing.T) {
	e := billingSource(t)
	src, err := e.fileSource("")
	require.NoError(t, err)
	raw, err := requestFromReflection(src, "billing.v1.RateSourceService", "CreateRateSource", nil, nil)
	require.NoError(t, err)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, map[string]interface{}{"operation_id": "1", "name": "qa", "kind": "KIND_MAIN", "active": false,
		"limit": map[string]interface{}{"units": "1", "currency": "qa"}, "starts_at": "2026-01-01T00:00:00Z", "priority": float64(1)}, got)
}

func TestRequestFromReflectionErrors(t *testing.T) {
	e := billingSource(t)
	src, err := e.fileSource("")
	require.NoError(t, err)
	_, err = requestFromReflection(src, "billing.v1.RateSourceService", "Missing", nil, nil)
	assert.ErrorContains(t, err, "has no method Missing")
	_, err = requestFromReflection(src, "billing.v1.RateSourceService", "CreateRateSource", map[string]string{"nope": "1"}, nil)
	assert.ErrorContains(t, err, "has no field nope")
	_, err = requestFromReflection(src, "billing.v1.RateSourceService", "CreateRateSource", map[string]string{"active": "maybe"}, nil)
	assert.ErrorContains(t, err, `fill active="maybe"`)
}
