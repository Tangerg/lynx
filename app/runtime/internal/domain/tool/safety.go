package tool

// SafetyClassFor maps a built-in tool name to its side-effect safety class. It
// is the single source of truth for the name→class mapping — consumed for the
// tools.list wire metadata AND by the approval gate ([approval.GateFor]) — so the
// two views never drift apart. Unknown tools (task, MCP, third-party tools) fall
// to Exec (fail-conservative: they may do anything). A future milestone may let
// users override per-tool via config.
func SafetyClassFor(name string) SafetyClass {
	switch name {
	case "read", "glob", "grep", "lsp", "lsp_diagnostics", "skill", "ask_user", "exit_plan_mode", "codebase_search", "sourcegraph_search":
		// lsp / lsp_diagnostics are read-only code-intelligence queries — same
		// class as read/glob/grep. skill only reads skill files. ask_user has no
		// side effect (it IS a HITL interrupt, so gating it would double-prompt).
		// exit_plan_mode is the way out of the read-only plan stance — it must
		// stay Safe or the agent would be trapped in plan mode.
		return SafetyClassSafe
	case "write", "edit", "apply_patch", "download", "schedule":
		return SafetyClassWrite
	default:
		return SafetyClassExec
	}
}

// ClassName is the wire vocabulary (API.md §4.4 SafetyClass: "safe" | "write" |
// "exec") for a class — the canonical string a client renders, stamped on the
// live toolCall Item and the approval prompt.
func ClassName(c SafetyClass) string {
	switch c {
	case SafetyClassSafe:
		return "safe"
	case SafetyClassWrite:
		return "write"
	default:
		return "exec"
	}
}
