package chat

import "github.com/Tangerg/lynx/lyra/internal/service/approval"

// gateAction is the three-way outcome of the per-call permission
// gate: run it, ask the user, or refuse outright. Replacing the
// older bool ("needs approval?") lets ModeReadOnly express "deny
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
//   - ModeReadOnly → deny every write / exec / unknown tool outright
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
	case approval.ModeReadOnly:
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
// engine doesn't see it — the chat service maps name → class here
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
// NOTE: tool/engine.go's defaultSafetyClass encodes the same name→class
// mapping for ListTools metadata; keep the shared rows in sync.
func safetyClassFor(name string) safetyClass {
	switch name {
	case "read", "grep", "glob", "skill", "ask_user":
		// skill only reads skill files (list / load / load_resource) — read-only
		// discovery, so it never needs an approval gate.
		// ask_user has no side effect — it IS a HITL interrupt itself (the
		// model asking the user a question), so it must never be approval-
		// gated on top: that would double-prompt. Its own interrupt handles
		// the human interaction.
		return safetyClassSafe
	case "write", "edit":
		return safetyClassWrite
	default:
		return safetyClassExec
	}
}
