// Package tool defines Lyra's registered-tool catalog and direct invocation
// surface. Clients enumerate available tools and, for diagnostics, may invoke
// one directly without going through a chat turn.
package tool

import "context"

// Tool is the metadata of one registered tool. Schema is the JSON Schema
// the model is shown; SafetyClass drives the default approval flow
// (see approval.Policy).
type Tool struct {
	Name        string
	Description string
	Schema      string
	SafetyClass SafetyClass
}

// SafetyClass classifies how aggressively the runtime gates a tool call
// behind an approval prompt. The classification is per-tool default;
// runtime mode (safe/balanced/yolo) may override.
type SafetyClass int

const (
	// SafetyClassSafe — read-only, no side effects (read, grep, glob,
	// skill). Never prompts. Network-reaching tools (webfetch etc.) are
	// NOT safe — they classify as Exec, fail-conservative.
	SafetyClassSafe SafetyClass = iota
	// SafetyClassWrite — writes files in cwd. Prompts in `safe` mode.
	SafetyClassWrite
	// SafetyClassExec — executes arbitrary commands (Shell). Prompts
	// in `safe` and `balanced` modes.
	SafetyClassExec
	// SafetyClassNetwork — reaches off-host network. Prompts when
	// configured.
	SafetyClassNetwork
)

// Catalog lists the registered model-facing tools.
type Catalog interface {
	// List returns every registered tool. Empty result is valid (no
	// tools registered).
	List(ctx context.Context) ([]Tool, error)
}

// Invoker runs registered tools directly, outside an agent turn.
type Invoker interface {
	// Invoke runs a tool directly outside a chat turn. Useful for
	// diagnostics and for clients that want to drive workflows
	// without the LLM in the loop. Returns the tool's raw output.
	Invoke(ctx context.Context, name string, arguments string) (string, error)
}

// Registry is the full registered-tool surface the runtime owns. Consumers
// should depend on [Catalog] or [Invoker] when they need only one side.
type Registry interface {
	Catalog
	Invoker
}
