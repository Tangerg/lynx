package tool

// RiskLevel is the coarse severity displayed when a tool call requires human
// approval. The empty value is invalid; callers may use it to mean no approval
// risk was attached.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// SafetyClassFor maps a built-in tool name to its side-effect safety class. It
// is the single source of truth for the name→class mapping — consumed for the
// tools.list wire metadata AND by the approval gate ([approval.GateFor]) — so the
// two views never drift apart. Unknown tools (MCP, third-party tools) fall
// to Exec (fail-conservative: they may do anything). A future milestone may let
// users override per-tool via config.
func SafetyClassFor(name string) SafetyClass {
	switch name {
	case "read", "glob", "grep", "lsp", "lsp_diagnostics", "skill", "ask_user", "exit_plan_mode", "codebase_search", "sourcegraph_search", "task":
		// lsp / lsp_diagnostics are read-only code-intelligence queries — same
		// class as read/glob/grep. skill only reads skill files. ask_user has no
		// side effect (it IS a HITL interrupt, so gating it would double-prompt).
		// exit_plan_mode is the way out of the read-only plan stance — it must
		// stay Safe or the agent would be trapped in plan mode. task is pure
		// orchestration; every child side effect is gated at the child tool.
		return SafetyClassSafe
	case "write", "edit", "apply_patch", "download", "schedule":
		return SafetyClassWrite
	default:
		return SafetyClassExec
	}
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
