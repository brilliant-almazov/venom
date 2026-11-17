package tempo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The service to fix is the deepest span that failed: a gateway answering 500 over a billing gRPC call that answered
// INTERNAL names svc-billing; a trace with no failed span names none.
func TestCulprit(t *testing.T) {
	failed := []byte(`{"batches":[
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-gateway"}}]},"scopeSpans":[{"spans":[
 {"spanId":"a","name":"POST /v1/billing/accounts","startTimeUnixNano":"1","status":{"code":2,"message":"internal"}}]}]},
{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-billing"}}]},"scopeSpans":[{"spans":[
 {"spanId":"b","parentSpanId":"a","name":"billing.v1.Accounts/Create","startTimeUnixNano":"2",
  "attributes":[{"key":"rpc.grpc.status_code","value":{"intValue":"13"}}]},
 {"spanId":"c","parentSpanId":"b","name":"SELECT accounts","startTimeUnixNano":"3"}]}]}]}`)
	service, span, ok := Culprit(failed)
	assert.True(t, ok)
	assert.Equal(t, "svc-billing", service)
	assert.Equal(t, "billing.v1.Accounts/Create", span)

	green := []byte(`{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc-gateway"}}]},` +
		`"scopeSpans":[{"spans":[{"spanId":"a","name":"GET /v1/x","startTimeUnixNano":"1",` +
		`"attributes":[{"key":"rpc.grpc.status_code","value":{"intValue":"0"}}]}]}]}]}`)
	_, _, ok = Culprit(green)
	assert.False(t, ok)
}
