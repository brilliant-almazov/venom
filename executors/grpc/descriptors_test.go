package grpc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSourceFromProtoFiles(t *testing.T) {
	dir := t.TempDir()
	proto := `syntax = "proto3";
package demo.v1;
message Ping { string id = 1; }
service DemoService { rpc Echo(Ping) returns (Ping); }
`
	if err := os.WriteFile(filepath.Join(dir, "demo.proto"), []byte(proto), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := Executor{ProtoFiles: []string{"demo.proto"}, ImportPaths: []string{"."}}.fileSource(dir)
	if err != nil || src == nil {
		t.Fatalf("source %v %v", src, err)
	}
	d, err := src.FindSymbol("demo.v1.DemoService")
	if err != nil || d.GetFullyQualifiedName() != "demo.v1.DemoService" {
		t.Fatalf("symbol %v %v", d, err)
	}
	if src, err := (Executor{}).fileSource(dir); src != nil || err != nil {
		t.Fatalf("no files must give reflection: %v %v", src, err)
	}
}
