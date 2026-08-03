package toolset

import (
	"encoding/json"
	"slices"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
)

type fileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}

func mutationPaths(tool toolcontract.Tool, arguments string) ([]string, error) {
	var paths []string
	reporter, ok, err := toolcontract.Capability[fileMutationReporter](tool)
	if err != nil {
		return nil, err
	}
	if ok {
		reported, err := reporter.MutationPaths(arguments)
		if err != nil {
			return nil, err
		}
		paths = append(paths, reported...)
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
	return cleanPathList(paths), nil
}

func resolvedMutationPaths(tool toolcontract.Tool, arguments, workdir string) ([]string, error) {
	paths, err := mutationPaths(tool, arguments)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		paths[i] = pathidentity.Canonical(workdir, path)
	}
	return cleanPathList(paths), nil
}

func cleanPathList(paths []string) []string {
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}
