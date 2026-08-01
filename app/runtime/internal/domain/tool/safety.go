package tool

import "slices"

// RiskLevel is the coarse severity displayed when a tool call requires human
// approval. The empty value is invalid; callers may use it to mean no approval
// risk was attached.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// safetyClasses is the name→class mapping for the built-in tools.
//
// A table rather than a switch because the set has to be enumerable: a name here
// that no built-in tool answers to is a safety policy for something nobody can
// call, and the only way to notice is to read the keys back (see the toolset's
// completeness guard). A switch's cases cannot be read at runtime, so nothing
// could check.
//
// Safe means "no side effect the user needs to approve":
//   - lsp / lsp_diagnostics are read-only code-intelligence queries — the same
//     class as read/glob/grep.
//   - skill only reads skill files.
//   - ask_user has no side effect: it IS a HITL interrupt, so gating it would
//     double-prompt.
//   - exit_plan_mode is the way out of the read-only plan stance — Exec or Write
//     here would trap the agent in plan mode.
//   - propose_skill only stages a draft, and gates promotion behind its own
//     human-approval interrupt (same double-prompt reasoning as ask_user).
//   - task is pure orchestration; every child side effect is gated at the child
//     tool.
var safetyClasses = map[string]SafetyClass{
	"read":               SafetyClassSafe,
	"glob":               SafetyClassSafe,
	"grep":               SafetyClassSafe,
	"lsp":                SafetyClassSafe,
	"lsp_diagnostics":    SafetyClassSafe,
	"skill":              SafetyClassSafe,
	"ask_user":           SafetyClassSafe,
	"exit_plan_mode":     SafetyClassSafe,
	"propose_skill":      SafetyClassSafe,
	"codebase_search":    SafetyClassSafe,
	"sourcegraph_search": SafetyClassSafe,
	NameReadToolResult:   SafetyClassSafe,
	"task":               SafetyClassSafe,

	"write":       SafetyClassWrite,
	"edit":        SafetyClassWrite,
	"apply_patch": SafetyClassWrite,
	"download":    SafetyClassWrite,
	"schedule":    SafetyClassWrite,
}

// SafetyClassFor maps a built-in tool name to its side-effect safety class. It
// is the single source of truth for the name→class mapping — consumed for the
// tools.list wire metadata AND by the approval gate ([approval.GateFor]) — so the
// two views never drift apart. Unknown tools (MCP, third-party tools) fall
// to Exec (fail-conservative: they may do anything).
func SafetyClassFor(name string) SafetyClass {
	if class, ok := safetyClasses[name]; ok {
		return class
	}
	return SafetyClassExec
}

// ClassifiedToolNames is every name the table states a class for. It exists so a
// test can check the mapping against the tools that actually exist; production
// reads classes through [SafetyClassFor], one name at a time.
func ClassifiedToolNames() []string {
	names := make([]string, 0, len(safetyClasses))
	for name := range safetyClasses {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Valid reports whether c is a defined safety class.
func (c SafetyClass) Valid() bool {
	switch c {
	case SafetyClassSafe, SafetyClassWrite, SafetyClassExec, SafetyClassNetwork:
		return true
	default:
		return false
	}
}

// Risk returns the conservative human-facing severity for c. An unknown class
// is high risk so an uninitialized or future value never weakens a prompt.
func (c SafetyClass) Risk() RiskLevel {
	switch c {
	case SafetyClassSafe:
		return RiskLow
	case SafetyClassWrite:
		return RiskMedium
	default:
		return RiskHigh
	}
}

// Valid reports whether r is a defined risk level.
func (r RiskLevel) Valid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}
