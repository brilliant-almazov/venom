package check

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
	"github.com/ovh/venom/internal/golden"
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

const ruleBody = `{"rules":[{"id":1,"literal":"shop","group":"g1","origin":"own"},{"id":2,"literal":"car","group":"g2","origin":"seed"},{"id":3,"literal":"shop","group":"g1","origin":"own"}]}`

func TestCheckFindAllObjects(t *testing.T) {
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": ruleBody, "find_all": `\{[^{}]*"literal":"shop"[^{}]*\}`})
	require.NoError(t, err)
	r := out.(Result)
	assert.Equal(t, 2, r.Count)
	golden.Text(t, "find_all_objects", r.Matches+"\n")
}

func TestCheckFindAllReplaceJoin(t *testing.T) {
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": ruleBody,
		"find_all": `\{[^{}]*"origin":"own"[^{}]*\}`, "replace": []string{`,"group":"[^"]*","origin":"[^"]*"`, ""}, "join": ","})
	require.NoError(t, err)
	r := out.(Result)
	golden.JSON(t, "find_all_replace_join", map[string]interface{}{"count": r.Count, "matches": r.Matches, "joined": r.Joined})
}

func TestCheckFindAllDuplicates(t *testing.T) {
	rows := "svc-a  KAFKA_URL  x\nsvc-a  LOG_LEVEL  y\nsvc-b  KAFKA_URL  z\nsvc-a  KAFKA_URL  w\nsvc-b  KAFKA_URL  v\n"
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": rows, "find_all": `(?m)^(\S+\s+\S+)`,
		"replace": []string{`\s+`, " "}, "duplicates": true})
	require.NoError(t, err)
	r := out.(Result)
	golden.JSON(t, "find_all_duplicates", map[string]interface{}{"count": r.Count, "matches": r.Matches, "duplicates": r.Duplicates})
}

func TestCheckFindAllBadInput(t *testing.T) {
	_, err := Executor{}.Run(context.Background(), venom.TestStep{"value": "x", "find_all": "("})
	assert.Error(t, err)
	_, err = Executor{}.Run(context.Background(), venom.TestStep{"value": "x", "find_all": "x", "replace": []string{"x"}})
	assert.Error(t, err)
}

func TestCheckEqualJSONSortLists(t *testing.T) {
	a := `{"protoVersion":"v1.4.0","grpcServices":["b.v1.B","a.v1.A",{"n":2,"m":1}]}`
	b := `{"grpcServices":[{"m":1,"n":2},"a.v1.A","b.v1.B"],"protoVersion":"v1.4.0"}`
	out, err := Executor{}.Run(context.Background(), venom.TestStep{"value": a, "equal_json": b, "sort_lists": true})
	require.NoError(t, err)
	assert.True(t, out.(Result).JSONEqual)

	out, err = Executor{}.Run(context.Background(), venom.TestStep{"value": a, "equal_json": b})
	require.NoError(t, err)
	r := out.(Result)
	assert.False(t, r.JSONEqual, "lists in another order differ without sort_lists")
	golden.Text(t, "equal_json_diff", r.JSONDiff+"\n")

	_, err = Executor{}.Run(context.Background(), venom.TestStep{"value": a, "equal_json": "{"})
	assert.Error(t, err)
}
