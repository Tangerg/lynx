package fs

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/core/tool"
)

// GlobRequest is the LLM-facing argument shape for the glob tool.
type GlobRequest struct {
	Pattern    string `json:"pattern" jsonschema:"minLength=1" jsonschema_description:"Doublestar path pattern, such as **/*.go or src/**/*.ts."`
	Path       string `json:"path,omitempty" jsonschema_description:"Directory to search under. Defaults to the workspace root."`
	IgnoreCase bool   `json:"ignore_case,omitempty" jsonschema_description:"Match path components case-insensitively. Default false."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Maximum paths to return. Defaults to 100 and cannot exceed 1000."`
}

// GlobResponse is the LLM-facing return shape.
type GlobResponse struct {
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated,omitempty"`
}

var _ toolcontract.Tool = (*GlobTool)(nil)

// GlobTool is the thin LLM-facing adapter for [Executor.Glob].
type GlobTool struct {
	executor Executor
	typed    toolcontract.Func[GlobRequest, GlobResponse]
}

// NewGlobTool builds a [GlobTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewGlobTool(executor Executor) *GlobTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	t := &GlobTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "glob",
			Description: "List file paths matching a doublestar pattern. Use patterns such as **/*.go or src/**/*.ts. " +
				"Use grep to search file contents.",
		},
		t.glob,
	)
	return t
}

func (g *GlobTool) Definition() chat.ToolDefinition {
	return g.typed.Definition()
}

// ConcurrencyKey opts glob into parallel execution — a read-only filename
// search has no conflict (the tool loop's optional concurrency contract).
func (g *GlobTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (g *GlobTool) Call(ctx context.Context, arguments string) (string, error) {
	return g.typed.Call(ctx, arguments)
}

func (g *GlobTool) glob(ctx context.Context, req GlobRequest) (GlobResponse, error) {
	res, err := g.executor.Glob(ctx, GlobInput{
		Pattern:    req.Pattern,
		Root:       req.Path,
		IgnoreCase: req.IgnoreCase,
		MaxResults: req.MaxResults,
	})
	if err != nil {
		return GlobResponse{}, fmt.Errorf("fs.glob: %w", err)
	}
	return res, nil
}
