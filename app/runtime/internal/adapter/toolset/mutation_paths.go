package toolset

import (
	"encoding/json"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
)

type mutatedPathReporter interface {
	MutatedPaths(arguments string) ([]string, error)
}

func mutatedPaths(tool chat.Tool, arguments string) []string {
	var paths []string
	if reporter, ok := tool.(mutatedPathReporter); ok {
		reported, err := reporter.MutatedPaths(arguments)
		if err == nil {
			paths = append(paths, reported...)
		}
	}
	if len(paths) == 0 {
		var a struct {
			Path string `json:"file_path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" {
			paths = append(paths, a.Path)
		}
	}
	return cleanPathList(paths)
}

func resolvedMutatedPaths(tool chat.Tool, arguments, workdir string) []string {
	paths := mutatedPaths(tool, arguments)
	for i, path := range paths {
		paths[i] = canonicalAbs(workdir, path)
	}
	return cleanPathList(paths)
}

func cleanPathList(paths []string) []string {
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
