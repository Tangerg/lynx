// Package tool defines the runtime's model-facing tool vocabulary.
package tool

// Tool is the metadata of one registered tool. Schema is the JSON Schema
// the model is shown; SafetyClass drives the default approval flow
// (see approval.RuntimePolicy).
type Tool struct {
	Name        string
	Description string
	Schema      Schema
	SafetyClass SafetyClass
}

// SafetyClass classifies how aggressively the runtime gates a tool call
// behind an approval prompt. Its values are also the durable vocabulary used
// by run checkpoints; the empty value is invalid rather than silently safe.
type SafetyClass string

const (
	// SafetyClassSafe — read-only, no side effects (read, grep, glob,
	// skill). Never prompts. Network-reaching tools (webfetch etc.) are
	// NOT safe — they classify as Exec, fail-conservative.
	SafetyClassSafe SafetyClass = "safe"
	// SafetyClassWrite — writes files in cwd. Prompts in `safe` mode.
	SafetyClassWrite SafetyClass = "write"
	// SafetyClassExec — executes arbitrary commands (Shell). Prompts
	// in `safe` and `balanced` modes.
	SafetyClassExec SafetyClass = "exec"
	// SafetyClassNetwork — reaches off-host network. Prompts when
	// configured.
	SafetyClassNetwork SafetyClass = "network"
)
