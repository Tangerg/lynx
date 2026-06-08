package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// GlobRequest is the LLM-facing argument shape for the glob tool.
type GlobRequest struct {
	Pattern    string `json:"pattern" jsonschema:"required" jsonschema_description:"Doublestar glob pattern. Examples: \"**/*.go\" (all Go files), \"src/**/*.ts\" (TS files under src), \"cmd/*/main.go\" (one level deep)."`
	Path       string `json:"path,omitempty" jsonschema_description:"Directory to search under. Defaults to the workspace root."`
	IgnoreCase bool   `json:"ignore_case,omitempty" jsonschema_description:"Match path components case-insensitively. Default false."`
	MaxResults int    `json:"max_results,omitempty" jsonschema_description:"Cap on results. 0 = use default cap (100)."`
}

// GlobResponse is the LLM-facing return shape.
type GlobResponse struct {
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated,omitempty"`
}

var globToolSchema, _ = pkgjson.StringDefSchemaOf(GlobRequest{})

var _ chat.Tool = (*GlobTool)(nil)

// GlobTool is the thin LLM-facing adapter for [Executor.Glob].
type GlobTool struct {
	executor Executor
}

// NewGlobTool builds a [GlobTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewGlobTool(executor Executor) *GlobTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &GlobTool{executor: executor}
}

func (t *GlobTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "glob",
		Description: "List file paths matching a doublestar pattern. " +
			"Examples: `**/*.go` (all Go files), `src/**/*.ts` (TS files under src), `cmd/*/main.go` (one level deep). " +
			"For searching file *contents*, use the grep tool instead.",
		InputSchema: globToolSchema,
	}
}

func (t *GlobTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *GlobTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req GlobRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.glob: parse arguments: %w", err)
	}
	if req.Pattern == "" {
		return "", fmt.Errorf("fs.glob: %w", ErrEmptyPattern)
	}
	res, err := t.executor.Glob(ctx, GlobInput{
		Pattern:    req.Pattern,
		Root:       req.Path,
		IgnoreCase: req.IgnoreCase,
		MaxResults: req.MaxResults,
	})
	if err != nil {
		return "", fmt.Errorf("fs.glob: %w", err)
	}
	body, err := json.Marshal(GlobResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.glob: marshal: %w", err)
	}
	return string(body), nil
}
