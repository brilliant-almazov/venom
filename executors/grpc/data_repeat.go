package grpc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/ovh/venom"
)

// DataRepeat sets the request field at Path (dot-separated) to Count items, item i being Template, a JSON text, with
// {{.index}} = i. The items are built inside the executor, so the request body never passes through a step var.
type DataRepeat struct {
	Path     string `json:"path" yaml:"path" mapstructure:"path"`
	Count    int    `json:"count" yaml:"count" mapstructure:"count"`
	Template string `json:"template" yaml:"template" mapstructure:"template"`
}

// Expand returns the request data with the repeated field set; the fields on the path are made when missing.
func (d DataRepeat) Expand(data []byte) ([]byte, error) {
	if d.Path == "" || d.Count < 0 {
		return nil, fmt.Errorf("grpc: data_repeat takes a path and a count of 0 or more")
	}
	tpl, err := template.New("data_repeat").Option("missingkey=error").Parse(d.Template)
	if err != nil {
		return nil, fmt.Errorf("grpc: data_repeat.template: %w", err)
	}
	root := map[string]interface{}{}
	if len(bytes.TrimSpace(data)) > 0 && string(bytes.TrimSpace(data)) != "null" {
		if err := venom.JSONUnmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("grpc: data_repeat over a request that is not a JSON object: %w", err)
		}
	}
	items := make([]interface{}, 0, d.Count)
	var buf bytes.Buffer
	for i := 0; i < d.Count; i++ {
		buf.Reset()
		if err := tpl.Execute(&buf, map[string]interface{}{"index": i}); err != nil {
			return nil, fmt.Errorf("grpc: data_repeat.template item %d: %w", i, err)
		}
		var item interface{}
		if err := venom.JSONUnmarshal(buf.Bytes(), &item); err != nil {
			return nil, fmt.Errorf("grpc: data_repeat.template item %d is not JSON: %w", i, err)
		}
		items = append(items, item)
	}
	keys := strings.Split(d.Path, ".")
	node := root
	for _, k := range keys[:len(keys)-1] {
		next, ok := node[k].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			node[k] = next
		}
		node = next
	}
	node[keys[len(keys)-1]] = items
	return json.Marshal(root)
}

// digest is the size in bytes and the sha256 of a request body.
func digest(data []byte) (int, string) {
	sum := sha256.Sum256(data)
	return len(data), hex.EncodeToString(sum[:])
}
