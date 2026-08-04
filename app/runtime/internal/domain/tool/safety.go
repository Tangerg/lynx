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
//   - lsp is a read-only code-intelligence query — the same
//     class as read/glob/grep.
//   - list_skills, load_skill, and read_skill_resource only read skill files.
//   - ask_user has no side effect: it IS a HITL interrupt, so gating it would
//     double-prompt.
//   - enter_plan_mode only narrows one session; set_plan changes session Plan state;
//     exit_plan_mode owns its own approval. Gating any of the three would make
//     Plan mode unusable or double-prompt.
//   - delegate_task is pure orchestration; every child side effect is gated at the child
//     tool.
//   - create_goal is itself the explicit autonomous-work opt-in the user asked
//     for; get_goal is read-only; report_goal_outcome only terminates that owned
//     loop. Gating those control operations would duplicate the intent gate.
//   - propose_skill only records the reusable workflow the user explicitly
//     requested as a pending proposal; it cannot activate the Skill. Gating the
//     submission would duplicate intent without protecting an active capability.
var safetyClasses = map[string]SafetyClass{
	"read":                 SafetyClassSafe,
	"glob":                 SafetyClassSafe,
	"grep":                 SafetyClassSafe,
	"lsp":                  SafetyClassSafe,
	"read_shell_output":    SafetyClassSafe,
	"list_schedules":       SafetyClassSafe,
	"list_skills":          SafetyClassSafe,
	"load_skill":           SafetyClassSafe,
	"read_skill_resource":  SafetyClassSafe,
	"search_memory":        SafetyClassSafe,
	"search_conversations": SafetyClassSafe,
	"search_tools":         SafetyClassSafe,
	"ask_user":             SafetyClassSafe,
	"enter_plan_mode":      SafetyClassSafe,
	"exit_plan_mode":       SafetyClassSafe,
	"set_plan":             SafetyClassSafe,
	NameReadToolResult:     SafetyClassSafe,
	"delegate_task":        SafetyClassSafe,
	"create_goal":          SafetyClassSafe,
	"get_goal":             SafetyClassSafe,
	"report_goal_outcome":  SafetyClassSafe,
	"propose_skill":        SafetyClassSafe,

	"write":           SafetyClassWrite,
	"edit":            SafetyClassWrite,
	"apply_patch":     SafetyClassWrite,
	"create_schedule": SafetyClassWrite,
	"delete_schedule": SafetyClassWrite,

	"shell":      SafetyClassExec,
	"stop_shell": SafetyClassExec,

	"web_fetch":    SafetyClassNetwork,
	"web_search":   SafetyClassNetwork,
	"http_request": SafetyClassNetwork,
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
