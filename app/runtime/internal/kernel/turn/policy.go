package turn

import "github.com/Tangerg/lynx/app/runtime/internal/domain/approval"

// gateAction is the three-way outcome of the per-call permission
// gate: run it, ask the user, or refuse outright. Replacing the
// older bool ("needs approval?") lets ModePlan express "deny
// without prompting" — a stance a bool can't capture.
type gateAction int

const (
	gatePass   gateAction = iota // run without prompting
	gatePrompt                   // register an approval request and wait
	gateDeny                     // refuse immediately, no prompt
)

// gateFor encodes the (tool, mode) → gate action. The rules mirror
// the strictness gradient documented on [approval.Mode]:
//
//   - ModeYolo     → always pass
//   - ModeBalanced → prompt only on exec class (bash + unknown)
//   - ModeSafe     → prompt on every write / exec / unknown tool
//   - ModePlan     → deny every write / exec / unknown tool outright (read-only)
//
// Pure function so transport adapters and tests can audit the
// matrix without touching the service impl.
func gateFor(toolName string, mode approval.Mode) gateAction {
	if mode == approval.ModeYolo {
		return gatePass
	}
	cls := safetyClassFor(toolName)
	if cls == safetyClassSafe {
		return gatePass // read-only tools never gate, in any mode
	}
	switch mode {
	case approval.ModePlan:
		return gateDeny
	case approval.ModeSafe:
		return gatePrompt // write + exec
	case approval.ModeBalanced:
		if cls == safetyClassExec {
			return gatePrompt
		}
		return gatePass // write/network auto-allow
	}
	return gatePass
}

// safetyClass is the per-tool side-effect classification. The
// engine doesn't see it — the turn service maps name → class here
// as part of the gate's policy.
type safetyClass int

const (
	safetyClassSafe  safetyClass = iota // read-only (read / grep / glob)
	safetyClassWrite                    // mutates the workspace (write / edit)
	safetyClassExec                     // runs arbitrary code / network (bash / httpreq)
)

// safetyClassFor maps a built-in tool name to its class, for approval
// gating. Unknown tools (incl. `task` delegation, MCP, online tools)
// fall into ExecClass — fail-conservative, since they may do anything.
//
// Two separate name→class mappings exist — this one for the approval gate
// and the one in [tool.defaultSafetyClass] for the ListTools metadata.
// They use different enum types but share the same rows; keep them
// in sync when adding tools.
func safetyClassFor(name string) safetyClass {
	switch name {
	case "read", "grep", "glob", "lsp", "lsp_diagnostics", "skill", "ask_user", "exit_plan_mode":
		// lsp / lsp_diagnostics are read-only code-intelligence queries
		// (definition / references / hover / symbols / diagnostics — no side
		// effects), so they belong with read/grep/glob. Without this they fall to
		// the exec default → DENIED in plan mode (where code investigation is the
		// whole point) and needlessly approval-prompted in safe mode.
		// skill only reads skill files (list / load / load_resource) — read-only
		// discovery, so it never needs an approval gate.
		// ask_user has no side effect — it IS a HITL interrupt itself (the
		// model asking the user a question), so it must never be approval-
		// gated on top: that would double-prompt. Its own interrupt handles
		// the human interaction.
		// exit_plan_mode MUST be safe: it is the way OUT of the read-only plan
		// stance, so a gate that denied it in ModePlan would trap the agent.
		return safetyClassSafe
	case "write", "edit":
		return safetyClassWrite
	default:
		return safetyClassExec
	}
}

// safetyClassName is the wire string for a tool's class (API.md §4.4 SafetyClass
// vocab: "safe" | "write" | "exec"), stamped on the live toolCall Item so a
// client shows the risk class without joining tools.list. The turn layer owns
// the policy vocab; the transport passes it through.
func safetyClassName(name string) string {
	switch safetyClassFor(name) {
	case safetyClassSafe:
		return "safe"
	case safetyClassWrite:
		return "write"
	default:
		return "exec"
	}
}

// approvalRisk maps a gated tool to the (risk, reason) an approval card shows —
// a coarser low/medium/high read of its safety class plus a one-line why. Only
// write / exec tools ever reach an approval prompt (safe never gates), so this
// is defined for those two classes.
func approvalRisk(name string) (risk, reason string) {
	if safetyClassFor(name) == safetyClassWrite {
		return "medium", "Modifies files in the workspace."
	}
	return "high", "Runs commands or accesses the network."
}
