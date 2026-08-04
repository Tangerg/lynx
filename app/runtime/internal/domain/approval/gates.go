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
//   - ModeBalanced → auto-allow known write/network; prompt on exec and unknown
//   - ModeSafe     → prompt on every write / network / exec / unknown tool
//   - ModePlan     → deny every write / network / exec / unknown tool (read-only)
//
// Read-only tools ([tool.SafetyClassSafe]) never gate, in any mode. Pure
// function so adapters and tests can audit the matrix without touching the
// runtime policy. The concrete tool owner supplies the class; this function
// only decides the gate from domain values.
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
		return GatePrompt // every non-safe class, including unknown
	case ModeBalanced:
		switch cls {
		case tool.SafetyClassWrite, tool.SafetyClassNetwork:
			return GatePass
		default:
			// Exec and unknown classes fail closed. In particular, the invalid
			// zero value must not inherit write's balanced-mode exemption.
			return GatePrompt
		}
	}
	// An unknown policy mode must not silently disable approval for a
	// side-effecting tool. Mode validation normally rejects this earlier; this
	// fallback keeps the pure policy function safe in isolation too.
	return GatePrompt
}
