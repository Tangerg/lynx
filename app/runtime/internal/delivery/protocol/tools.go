package protocol

import "context"

// Tools is the tools.* method group. These methods expose the deliberately
// small direct-diagnostics surface; agent-run tools remain owned by Runs.
type Tools interface {
	ListTools(ctx context.Context, q PageQuery) (*Page[ToolSpec], error)
	InvokeTool(ctx context.Context, in InvokeToolRequest) (any, error)
}

// ToolSpec is one direct-invocation capability (API.md §4.7). It is not the
// agent's complete tool catalog: direct calls are limited to tools that can run
// without a session, process, approval flow, or model loop.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`  // JSON Schema
	SafetyClass SafetyClass    `json:"safetyClass,omitempty"` // see SafetyClass
}

// InvokeToolRequest — tools.invoke body (API.md §7.6). Cwd is the admitted
// workspace root for this direct diagnostic call; filesystem arguments must
// remain within it.
type InvokeToolRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Cwd       string         `json:"cwd,omitempty"`
}
