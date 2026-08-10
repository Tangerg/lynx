package protocol

// ToolSpec is one direct-invocation capability (API.md §4.7). It is not the
// agent's complete tool catalog: direct calls are limited to tools that can run
// without a session, process, approval flow, or model loop.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`  // JSON Schema
	SafetyClass SafetyClass    `json:"safetyClass,omitempty"` // see SafetyClass
}

// InvokeToolRequest — tools.invoke body (API.md §7.6). Workspace is optional for
// diagnostics that do not touch files; when present, filesystem arguments must
// remain within its admitted root.
type InvokeToolRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Workspace *WorkspaceRef  `json:"workspace,omitempty"`
}
