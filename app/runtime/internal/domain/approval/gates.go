package approval

import "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"

// GateAction is the three-way outcome of the per-call permission gate: run it,
// ask the user, or refuse outright. Replacing the older bool ("needs approval?")
// lets ModePlan express "deny without prompting" — a stance a bool can't capture.
type GateAction int

const (
	// GatePass runs the tool without prompting.
	GatePass GateAction = iota
	// GatePrompt registers an approval request and waits for a human decision.
	GatePrompt
	// GateDeny refuses outright with no prompt (ModePlan's read-only stance).
	GateDeny
)

// GateFor encodes the (tool-class, mode) → gate action. The rules mirror the
// strictness gradient documented on [Mode]:
//
//   - ModeYolo     → always pass
//   - ModeBalanced → prompt only on exec class (shell + unknown)
//   - ModeSafe     → prompt on every write / exec / unknown tool
//   - ModePlan     → deny every write / exec / unknown tool outright (read-only)
//
// Read-only tools ([tool.SafetyClassSafe]) never gate, in any mode. Pure
// function so transport adapters and tests can audit the matrix without
// touching the service impl. The caller classifies the tool name
// ([tool.SafetyClassFor]); this function only decides the gate from the class.
func GateFor(cls tool.SafetyClass, mode Mode) GateAction {
	if mode == ModeYolo {
		return GatePass
	}
	if cls == tool.SafetyClassSafe {
		return GatePass // read-only tools never gate, in any mode
	}
	switch mode {
	case ModePlan:
		return GateDeny
	case ModeSafe:
		return GatePrompt // write + exec
	case ModeBalanced:
		if cls == tool.SafetyClassExec {
			return GatePrompt
		}
		return GatePass // write/network auto-allow
	}
	return GatePass
}

// RiskFor maps a gated tool's class to the (risk, reason) an approval card
// shows — a coarser low/medium/high read plus a one-line why. Only write / exec
// tools ever reach an approval prompt (safe never gates), so this is defined
// for those two classes.
func RiskFor(cls tool.SafetyClass) (risk, reason string) {
	if cls == tool.SafetyClassWrite {
		return "medium", "Modifies files in the workspace."
	}
	return "high", "Runs commands or accesses the network."
}
