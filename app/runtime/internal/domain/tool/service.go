// Package tool defines the ToolService — Lyra's tool registry surface.
// Clients enumerate available tools and (for diagnostics) invoke them
// directly without going through a chat turn.
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

// Service is the ToolService contract. The in-package New(eng)
// constructor returns an engine-backed implementation — list +
// invoke route through the engine's registered tool set.
type Service interface {
	// List returns every registered tool. Empty result is valid (no
	// tools registered).
	List(ctx context.Context) ([]Tool, error)

	// Invoke runs a tool directly outside a chat turn. Useful for
	// diagnostics and for clients that want to drive workflows
	// without the LLM in the loop. Returns the tool's raw output.
	Invoke(ctx context.Context, name string, arguments string) (string, error)
}
