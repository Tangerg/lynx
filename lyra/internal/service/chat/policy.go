package chat

import "github.com/Tangerg/lynx/lyra/internal/service/approval"

// needsApproval encodes the (tool, mode) → gating decision. The
// rules mirror the docs on [approval.Mode]:
//
//   - ModeYolo    → never gate
//   - ModeBalanced → gate only "exec" class (bash + unknown)
//   - ModeSafe     → gate every write / exec / unknown tool
//
// Pure function so transport adapters and tests can audit the
// matrix without touching the service impl.
func needsApproval(toolName string, mode approval.Mode) bool {
	if mode == approval.ModeYolo {
		return false
	}
	cls := safetyClassFor(toolName)
	switch mode {
	case approval.ModeSafe:
		return cls != safetyClassSafe
	case approval.ModeBalanced:
		return cls == safetyClassExec
	}
	return false
}

// safetyClass is the per-tool side-effect classification. The
// engine doesn't see it — chat.impl maps name → class as part of
// the gate's policy.
type safetyClass int

const (
	safetyClassSafe  safetyClass = iota // read-only (read / grep / glob)
	safetyClassWrite                    // mutates the workspace (write / edit)
	safetyClassExec                     // runs arbitrary code / network (bash / httpreq)
)

// safetyClassFor maps a built-in tool name to its class. Unknown
// tools fall into ExecClass — fail-conservative, since user-added
// tools may do anything.
func safetyClassFor(name string) safetyClass {
	switch name {
	case "read", "grep", "glob":
		return safetyClassSafe
	case "write", "edit":
		return safetyClassWrite
	default:
		return safetyClassExec
	}
}
