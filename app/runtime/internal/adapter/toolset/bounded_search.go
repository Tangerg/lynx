package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	workspaceadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

const (
	defaultRuntimeSearchResults  = 100
	maxRuntimeSearchResults      = 1000
	maxRuntimeSearchPatternBytes = workspaceapp.MaxGrepQueryBytes
	maxRuntimeGlobPatternBytes   = 4 << 10
	maxRuntimeSearchPathBytes    = 4 << 10
	maxRuntimeSearchResultBytes  = 1 << 20
	// Keys, punctuation, total, and truncated stay below this fixed reserve;
	// every variable JSON string or row is charged exactly below.
	runtimeSearchFramingBytes = 128
)

var errRuntimeSearchResultTooLarge = errors.New("toolset: search result exceeds its resource limit")

// Model search deliberately exposes only composable local primitives. Regex
// flags belong in the RE2 pattern, content context comes from read, and glob is
// the file-set primitive; process-specific rg modes therefore are not part of
// this contract.
type runtimeGrepRequest struct {
	Pattern    string `json:"pattern" jsonschema:"minLength=1,maxLength=65536" jsonschema_description:"Go/RE2 regular expression matched independently against each complete text line. Use inline flags such as (?i) for case-insensitive matching."`
	Path       string `json:"path,omitempty" jsonschema:"maxLength=4096" jsonschema_description:"Workspace-relative file or directory to search. Defaults to the workspace root."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Maximum complete matching lines to retain. Defaults to 100; total still reports the exact match count."`
}

type runtimeGlobRequest struct {
	Pattern    string `json:"pattern" jsonschema:"minLength=1,maxLength=4096" jsonschema_description:"Workspace-relative doublestar path pattern, such as **/*.go or src/**/*.ts."`
	Path       string `json:"path,omitempty" jsonschema:"maxLength=4096" jsonschema_description:"Workspace-relative file or directory to search under. Defaults to the workspace root."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Maximum complete paths to retain. Defaults to 100; total still reports the exact match count."`
}

type runtimeSearchResponse struct {
	Matches   []runtimeSearchResult `json:"matches"`
	Total     int                   `json:"total"`
	Truncated bool                  `json:"truncated"`
}

type runtimeSearchResult struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type runtimePathSearchResponse struct {
	Paths     []string `json:"paths"`
	Total     int      `json:"total"`
	Truncated bool     `json:"truncated"`
}

type concurrentSearchTool struct{ toolcontract.Tool }

func (concurrentSearchTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

type runtimeSearchTools struct {
	glob toolcontract.Tool
	grep toolcontract.Tool
}

func newRuntimeSearchTools(root string) runtimeSearchTools {
	glob := mustRuntimeSearchFunc(
		toolcontract.FuncConfig{
			Name: tool.Glob,
			Description: "List files from the finite, ignore-aware workspace catalog using a doublestar pattern. " +
				"Use grep to search file contents.",
		},
		func(ctx context.Context, request runtimeGlobRequest) (runtimePathSearchResponse, error) {
			return runtimeGlob(ctx, root, request)
		},
	)
	grep := mustRuntimeSearchFunc(
		toolcontract.FuncConfig{
			Name: tool.Grep,
			Description: "Search the finite, ignore-aware workspace text corpus with a Go/RE2 line regular expression. " +
				"Use glob to choose files and read to inspect surrounding lines.",
		},
		func(ctx context.Context, request runtimeGrepRequest) (runtimeSearchResponse, error) {
			return runtimeGrep(ctx, root, request)
		},
	)
	return runtimeSearchTools{
		glob: concurrentSearchTool{Tool: glob},
		grep: concurrentSearchTool{Tool: grep},
	}
}

func mustRuntimeSearchFunc[Input, Output any](
	config toolcontract.FuncConfig,
	call func(context.Context, Input) (Output, error),
) toolcontract.Func[Input, Output] {
	result, err := toolcontract.NewFunc(config, call)
	if err != nil {
		panic(err)
	}
	return result
}

func runtimeGlob(ctx context.Context, root string, request runtimeGlobRequest) (runtimePathSearchResponse, error) {
	if cause := context.Cause(ctx); cause != nil {
		return runtimePathSearchResponse{}, cause
	}
	if err := validateRuntimeSearchText(request.Pattern, maxRuntimeGlobPatternBytes, "glob pattern"); err != nil {
		return runtimePathSearchResponse{}, err
	}
	path, err := runtimeSearchPath(root, request.Path)
	if err != nil {
		return runtimePathSearchResponse{}, err
	}
	limit := normalizedRuntimeSearchLimit(request.MaxResults)
	entries, err := workspaceadapter.ListFiles(ctx, root, workspaceadapter.ListFilesOptions{
		Path: path, Glob: request.Pattern, Recursive: true,
	})
	if err != nil {
		if errors.Is(err, workspaceadapter.ErrListingTooLarge) {
			return runtimePathSearchResponse{}, errRuntimeSearchResultTooLarge
		}
		return runtimePathSearchResponse{}, fmt.Errorf("toolset: glob workspace: %w", err)
	}

	response := runtimePathSearchResponse{Paths: []string{}}
	material := runtimeSearchFramingBytes
	retain := true
	for _, entry := range entries {
		if entry.Kind != workspaceadapter.EntryFile {
			continue
		}
		response.Total++
		if !retain {
			continue
		}
		if len(response.Paths) >= limit {
			retain = false
			continue
		}
		encoded, marshalErr := json.Marshal(entry.Path)
		if marshalErr != nil {
			return runtimePathSearchResponse{}, fmt.Errorf("toolset: encode glob path: %w", marshalErr)
		}
		if len(encoded)+1 > maxRuntimeSearchResultBytes-material {
			retain = false
			continue
		}
		response.Paths = append(response.Paths, entry.Path)
		material += len(encoded) + 1
	}
	response.Truncated = response.Total > len(response.Paths)
	return response, nil
}

func runtimeGrep(ctx context.Context, root string, request runtimeGrepRequest) (runtimeSearchResponse, error) {
	if cause := context.Cause(ctx); cause != nil {
		return runtimeSearchResponse{}, cause
	}
	if err := validateRuntimeSearchText(request.Pattern, maxRuntimeSearchPatternBytes, "grep pattern"); err != nil {
		return runtimeSearchResponse{}, err
	}
	pattern, err := regexp.Compile(request.Pattern)
	if err != nil {
		return runtimeSearchResponse{}, fmt.Errorf("toolset: invalid grep pattern: %w", err)
	}
	path, err := runtimeSearchPath(root, request.Path)
	if err != nil {
		return runtimeSearchResponse{}, err
	}
	result, err := (workspaceadapter.FileBrowser{}).Grep(ctx, root, workspaceapp.GrepPlan{
		Path: path, Pattern: pattern, Limit: normalizedRuntimeSearchLimit(request.MaxResults),
	})
	if err != nil {
		return runtimeSearchResponse{}, fmt.Errorf("toolset: grep workspace: %w", err)
	}
	response := runtimeSearchResponse{
		Matches: make([]runtimeSearchResult, 0, len(result.Matches)),
		Total:   result.Total,
	}
	material := runtimeSearchFramingBytes
	for _, match := range result.Matches {
		row := runtimeSearchResult{
			Path: match.Path, Line: match.LineNumber, Text: match.Text,
		}
		encoded, marshalErr := json.Marshal(row)
		if marshalErr != nil {
			return runtimeSearchResponse{}, fmt.Errorf("toolset: encode grep row: %w", marshalErr)
		}
		if len(encoded)+1 > maxRuntimeSearchResultBytes-material {
			break
		}
		response.Matches = append(response.Matches, row)
		material += len(encoded) + 1
	}
	response.Truncated = response.Total > len(response.Matches)
	return response, nil
}

func normalizedRuntimeSearchLimit(requested int) int {
	if requested <= 0 {
		return defaultRuntimeSearchResults
	}
	return min(requested, maxRuntimeSearchResults)
}

func validateRuntimeSearchText(value string, limit int, field string) error {
	if value == "" {
		return fmt.Errorf("toolset: %s is required", field)
	}
	if len(value) > limit || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("toolset: invalid %s", field)
	}
	return nil
}

func runtimeSearchPath(root, requested string) (string, error) {
	if len(requested) > maxRuntimeSearchPathBytes || !utf8.ValidString(requested) || strings.ContainsRune(requested, 0) {
		return "", fmt.Errorf("toolset: invalid search path")
	}
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("toolset: search root must be absolute")
	}
	canonicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return "", fmt.Errorf("toolset: resolve search root: %w", err)
	}
	if requested == "" {
		return "", nil
	}
	target, err := pathidentity.Resolve(canonicalRoot, requested)
	if err != nil {
		return "", fmt.Errorf("toolset: resolve search path %q: %w", requested, err)
	}
	inside, err := pathidentity.Contains(canonicalRoot, target)
	if err != nil {
		return "", fmt.Errorf("toolset: compare search path %q: %w", requested, err)
	}
	if !inside {
		return "", fmt.Errorf("%w: %q", workspaceapp.ErrPathOutsideRoot, requested)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		return "", fmt.Errorf("toolset: inspect search path %q: %w", requested, statErr)
	}
	relative, err := filepath.Rel(canonicalRoot, target)
	if err != nil {
		return "", fmt.Errorf("toolset: relativize search path %q: %w", requested, err)
	}
	if relative == "." {
		return "", nil
	}
	return filepath.ToSlash(relative), nil
}
