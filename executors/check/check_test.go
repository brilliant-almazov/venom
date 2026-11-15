package check

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

func TestCheckValueAndJSON(t *testing.T) {
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": `{"code":200,"items":[1]}`})
	require.NoError(t, err)
	r := out.(Result)
	assert.Equal(t, `{"code":200,"items":[1]}`, r.Value)
	assert.Equal(t, map[string]interface{}{"code": json.Number("200"), "items": []interface{}{json.Number("1")}}, r.ValueJSON)
}

func TestCheckPlainValue(t *testing.T) {
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": "abc"})
	require.NoError(t, err)
	assert.Equal(t, Result{Value: "abc"}, out.(Result), "a string that is not JSON has no valuejson")
}

func TestCheckNumberValue(t *testing.T) {
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": 200})
	require.NoError(t, err)
	assert.Equal(t, "200", out.(Result).Value, "a value given as a number is its string")
}
