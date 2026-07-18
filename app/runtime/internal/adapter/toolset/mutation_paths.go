package toolset

import (
	"encoding/json"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
	"github.com/Tangerg/lynx/tools"
)

func mutationPaths(tool tools.Tool, arguments string) []string {
	var paths []string
	if reporter, ok := tool.(tools.FileMutationReporter); ok {
		reported, err := reporter.MutationPaths(arguments)
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

func resolvedMutationPaths(tool tools.Tool, arguments, workdir string) []string {
	paths := mutationPaths(tool, arguments)
	for i, path := range paths {
		paths[i] = pathidentity.Canonical(workdir, path)
	}
	return cleanPathList(paths)
}

func cleanPathList(paths []string) []string {
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
