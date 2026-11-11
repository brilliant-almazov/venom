package grpc

import (
	"fmt"
	"path/filepath"

	"github.com/fullstorydev/grpcurl"
)

func abs(dir string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		if filepath.IsAbs(p) || dir == "" {
			out[i] = p
		} else {
			out[i] = filepath.Join(dir, p)
		}
	}
	return out
}

// fileSource is the descriptor source of protoset or proto_files; nil when neither is set (reflection is used).
func (e Executor) fileSource(workdir string) (grpcurl.DescriptorSource, error) {
	switch {
	case len(e.Protoset) > 0:
		src, err := grpcurl.DescriptorSourceFromProtoSets(abs(workdir, e.Protoset)...)
		if err != nil {
			return nil, fmt.Errorf("grpc: protoset: %w", err)
		}
		return src, nil
	case len(e.ProtoFiles) > 0:
		src, err := grpcurl.DescriptorSourceFromProtoFiles(abs(workdir, e.ImportPaths), e.ProtoFiles...)
		if err != nil {
			return nil, fmt.Errorf("grpc: proto_files: %w", err)
		}
		return src, nil
	}
	return nil, nil
}
