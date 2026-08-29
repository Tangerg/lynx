package fs

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type GlobRequest struct {
	Pattern    string `json:"pattern" jsonschema:"minLength=1" jsonschema_description:"Doublestar path pattern, such as **/*.go or src/**/*.ts."`
	Path       string `json:"path,omitempty" jsonschema_description:"Directory to search under. Defaults to the workspace root."`
	IgnoreCase bool   `json:"ignore_case,omitempty" jsonschema_description:"Match path components case-insensitively. Default false."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"minimum=1,maximum=1000" jsonschema_description:"Maximum paths to return. Defaults to 100 and cannot exceed 1000."`
}

type GlobResponse struct {
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated,omitempty"`
}

var _ toolcontract.Tool = (*GlobTool)(nil)

type GlobTool struct {
	executor Globber
	typed    toolcontract.Func[GlobRequest, GlobResponse]
}

func NewGlobTool(executor Globber) *GlobTool {
	if isNilBackend(executor) {
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
func (g *GlobTool) ConcurrencyKey(toolcontract.Invocation) (key string, concurrent bool) {
	return "", true
}

func (g *GlobTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	return g.typed.Call(ctx, invocation)
}

func (g *GlobTool) glob(ctx context.Context, req GlobRequest) (GlobResponse, error) {
	res, err := g.executor.Glob(ctx, GlobInput(req))
	if err != nil {
		return GlobResponse{}, fmt.Errorf("fs.glob: %w", err)
	}
	return res, nil
}
