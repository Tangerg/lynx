package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// agentTool is the shared [chat.Tool] wrapper used by every
// "expose an agent as a tool" flow — typed [AsChatTool] /
// [AsChatToolFromAgent] / [AsMCPTool] and dynamic
// [AllAchievableTools] / [PublishAll]. The flow is identical across
// callers; only three strategies vary:
//
//   - decode: how to turn the LLM's JSON arguments into a value to
//     bind onto the child blackboard (typed json.Unmarshal vs
//     reflection-driven typed-via-sample).
//   - run: how to start the agent process (SpawnChildProtectedOnly for
//     supervisor flow, RunFresh for top-level / MCP-publish).
//   - extract: how to pull the result from the completed child
//     blackboard (typed [core.ResultOfType] vs untyped
//     [core.LastResultBindingName] lookup).
//
// Definition / Metadata / Call shape is the same for every caller; the
// strategies are closed over at construction so the per-Call hot path
// is just three function calls.
type agentTool struct {
	platform *Platform // drops the terminal child's process + snapshot (see discard)
	def      chat.ToolDefinition
	label    string // surfaces in error messages — "subagent" / "publish agent"
	agent    *core.Agent
	decode   func(arguments string) (any, error)
	run      func(ctx context.Context, in any) (*AgentProcess, error)
	extract  func(child *AgentProcess) (any, error)
}

func (t *agentTool) Definition() chat.ToolDefinition { return t.def }
func (t *agentTool) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }

// ConcurrencyKey opts an agent-as-tool into parallel execution (the tool loop's
// optional concurrency contract): each call spawns an ISOLATED child process
// (its own blackboard + session, no shared mutable state), and a child that
// parks for HITL surfaces as a {"status":"waiting"} result rather than
// interrupting the parent round — so the driver may run several sub-agent
// delegations (e.g. `task`) concurrently.
func (t *agentTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *agentTool) Call(ctx context.Context, arguments string) (string, error) {
	in, err := t.decode(arguments)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, t.agent.Name, err)
	}

	proc, err := t.run(ctx, in)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, t.agent.Name, err)
	}

	// Waiting is special — surface as JSON tool result instead of an
	// error so the calling LLM can decide to drop the path or re-plan.
	// All other non-Completed statuses bubble up via TerminalError.
	if proc.Status() == core.StatusWaiting {
		// Parked for HITL: the host resumes it via the returned process_id, so
		// its snapshot MUST survive — do NOT discard here.
		return waitingResultText(t.agent.Name, proc), nil
	}
	// Terminal from here on: the child is dead weight once its result is read,
	// so release it (registry + persisted snapshot). Registered after the
	// Waiting check so a parked child is never discarded.
	defer t.discard(ctx, proc)
	if err = proc.TerminalError(); err != nil {
		return "", fmt.Errorf("%s %q (process %q): %w", t.label, t.agent.Name, proc.ID(), err)
	}

	out, err := t.extract(proc)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, t.agent.Name, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("%s %q: marshal output: %w", t.label, t.agent.Name, err)
	}
	return string(encoded), nil
}

// discard releases a TERMINATED child: drop it from the platform registry and
// delete its persisted snapshot. With a ProcessStore wired the runtime
// auto-snapshots every tick (terminal completion included), but a terminal
// child's snapshot is dead weight — left behind, a parent that spawns many
// sub-agents accumulates one orphaned snapshot row per call. Best-effort:
// cleanup failures don't affect the already-finished call. NEVER call it on a
// StatusWaiting child — that snapshot must survive for HITL resume.
func (t *agentTool) discard(ctx context.Context, child *AgentProcess) {
	if t.platform == nil {
		return
	}
	id := child.ID()
	_ = t.platform.RemoveProcess(id)
	if store := t.platform.ProcessStore(); store != nil {
		_ = store.Delete(ctx, id)
	}
}

// waitingResultText renders a JSON description of a sub-agent's
// pending await as a tool-result string. The parent LLM sees:
//
//	{"status":"waiting", "agent":"…", "process_id":"…",
//	 "awaitable_id":"…", "prompt":<payload>}
//
// "prompt" is whatever [core.Awaitable.PromptAny] returns — typically
// the human-facing payload of a [hitl.TypedRequest]. Hosts can drive
// the child to completion via [Platform.ResumeProcess] +
// [Platform.ContinueProcess] using the returned process_id; the
// returned text is informational, suited for the LLM's next-turn
// reasoning.
func waitingResultText(agentName string, child *AgentProcess) string {
	payload := map[string]any{
		"status":     "waiting",
		"agent":      agentName,
		"process_id": child.ID(),
	}
	if a := child.PendingAwaitable(); a != nil {
		payload["awaitable_id"] = a.ID()
		payload["prompt"] = a.PromptAny()
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// Fallback to a plain sentence if marshal somehow fails — keeps
		// the LLM-visible result useful even in degenerate cases.
		return fmt.Sprintf(`{"status":"waiting","agent":%q,"process_id":%q}`, agentName, child.ID())
	}
	return string(encoded)
}
