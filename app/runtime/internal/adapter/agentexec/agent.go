package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
)

// turnInput is the typed input to the chat turn agent — the user's message
// plus the per-turn provider selection and any image attachments.
type turnInput struct {
	Message string

	// Provider is the turn's provider id (the per-run selection; empty for a
	// default turn). Carried so per-round cost pricing can attribute spend to
	// the right provider — a model id alone is ambiguous across providers.
	Provider string

	// Media carries the turn's image attachments, attached to the opening
	// user message as UserMessage.Media. Nil for a text-only turn (and for
	// `task` sub-agents, whose prompt is text).
	Media []*media.Media

	// Cwd is the working directory the turn's filesystem + shell tools run
	// in. The chat action binds it protected on the blackboard so
	// the tool resolver anchors the tools there and `task` sub-agents inherit
	// it. Empty falls back to the engine's default workdir.
	Cwd string

	// SessionID anchors the turn to its session; the chat action binds it
	// protected so the read/edit guards can key file-read state per session
	// (same blackboard seam as Cwd). Empty for a sessionless smoke run.
	SessionID string

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. When
	// exceeded the action stops cleanly after the current round —
	// before paying for the next LLM call — and reports the partial
	// reply with [TurnOutput.StopReason] set to [StopReasonBudget].
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost the same way MaxBudget caps
	// tokens (the lynx analog of the SDK's maxBudgetUsd). 0 means no cost
	// cap. Requires a [Config.Pricing] hook — without one cost stays 0
	// and this never trips. Either ceiling stops the turn.
	MaxCostUSD float64

	// MaxSteps caps the number of tool-call rounds (model turns) the turn may
	// run. 0 means unlimited (bounded only by the tool loop's own iteration
	// cap). When reached the action stops cleanly after the round — before the
	// next LLM call — with [TurnOutput.StopReason] set to [StopReasonSteps].
	MaxSteps int

	// Options carries per-run generation tuning. It deliberately does not carry
	// model selection; Provider / per-run ChatClient own that boundary.
	Options *chat.Options
}

// StopReason identifies why an otherwise successful turn stopped before or at
// its final interaction boundary.
type StopReason string

const (
	// StopReasonNone means the interaction reached a tagged final event.
	StopReasonNone StopReason = ""
	// StopReasonBudget means token or cost limits stopped continuation.
	StopReasonBudget StopReason = "budget"
	// StopReasonSteps means the tool-call-round limit stopped continuation.
	StopReasonSteps StopReason = "steps"
)

// Valid reports whether r is a supported turn stop reason.
func (r StopReason) Valid() bool {
	switch r {
	case StopReasonNone, StopReasonBudget, StopReasonSteps:
		return true
	default:
		return false
	}
}

// TurnOutput is the typed output of one turn. Reply is the assistant's
// final text. Usage / UsageByModel / CostUSD are read back from the
// process budget — the agent framework's invocation ledger — rather
// than a hand-rolled tally: managed interaction records each LLM round,
// and these fields are the rolled-up view.
type TurnOutput struct {
	Reply string
	Usage accounting.TokenUsage

	// UsageByModel breaks Usage down per served model — the lynx analog
	// of the SDK's modelUsage. One entry for a plain single-model turn;
	// several once a turn spans models (tool rounds routed elsewhere,
	// sub-agents).
	UsageByModel []accounting.ModelUsage

	// CostUSD is the turn's total dollar cost, summed from the recorded
	// invocations. Zero unless an [accounting.Pricing] func is configured (providers
	// don't return a dollar figure on the chat path); see [Config.Pricing].
	CostUSD float64

	// StopReason is empty on normal completion, "budget" when token or cost
	// limits stopped continuation, and "steps" when the tool-call-round limit
	// stopped continuation. Reply holds partial streamed text for the two
	// artificial-stop cases.
	StopReason StopReason
}

// buildTurnAgent constructs the chat agent owned by this Engine.
// The Action's closure captures `e` so it can reach the engine's
// memory store for system-prompt composition without an extra
// parameter passed through every turn.
//
// The Action declares [toolport.ToolRoleCoding] so the runtime resolves the
// coding tool group at dispatch time; the body calls
// [core.ProcessContext.Interact], the framework-managed interaction boundary.
// Runtime owns model/tool iteration, checkpointing, suspension, usage, and
// limits; the app supplies its prompt, streaming projection, pricing, and
// product tool policy. The model can therefore call read / write / edit / glob /
// grep / shell freely within one turn without an app-owned loop.
//
// The body uses Stream rather than Call so each text chunk surfaces
// to [toolObserver.OnMessageDelta] as it arrives — transport
// adapters get a real streaming experience instead of one pre-buffered
// MessageDelta. Tool-call rounds still go through the same tool loop; tool
// events surface via the tool-decorator path independently of the text-delta
// path.
func (e *Engine) buildTurnAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "chat-agent", Description: "single-turn LLM chat with the default coding tool set", Actions: []agent.Action{agent.NewAction("chat", func(ctx context.Context, pc *core.ProcessContext, in turnInput) (TurnOutput, error) {
		if in.Cwd != "" {
			pc.Blackboard().StoreProtected(turnctx.CwdBindingKey, in.Cwd)
		}
		if in.SessionID != "" {
			pc.Blackboard().StoreProtected(turnctx.SessionBindingKey, in.SessionID)
		}
		return e.runTurn(ctx, pc, in.Provider, in.Message, in.Media, in.Options, accounting.Budget{MaxTokens: in.MaxBudget, MaxCostUSD: in.MaxCostUSD, MaxSteps: in.MaxSteps})
	}, core.ActionConfig{ToolGroups: []core.ToolGroupRequirement{core.RequireToolGroup(toolport.ToolRoleCoding)}})}, Goals: []*agent.Goal{agent.NewOutputGoal[TurnOutput](core.GoalConfig{Description: "single-turn reply produced"})}})
}

// taskInput is the argument schema the model fills to call the `task`
// tool: one self-contained subtask description. lyra runs it in a fresh
// sub-agent (isolated context, the coding tools minus `task`) and hands
// back the sub-agent's final reply.
type taskInput struct {
	// Description is a short (3-5 word) label for the subtask, shown in the UI
	// while it runs. Display-only: it rides in the tool-call arguments for the
	// frontend, not consumed server-side (the sub-agent works from Prompt).
	Description string `json:"description" jsonschema_description:"Short (3-5 word) description of the subtask, shown in the UI while it runs."`
	Prompt      string `json:"prompt" jsonschema_description:"The full, self-contained instructions for the sub-agent — it does not see the main conversation, so include everything it needs."`
}

// SubagentDescription returns the task label surfaced to lifecycle hooks.
func (in taskInput) SubagentDescription() string { return in.Description }

// SubagentPrompt returns the task prompt surfaced to lifecycle hooks.
func (in taskInput) SubagentPrompt() string { return in.Prompt }

// buildSubtaskAgent constructs the agent behind the `task` delegation
// tool. Same chat body as the main agent, but: (1) named "task" so the
// derived tool is `task`; (2) declares [toolport.ToolRoleSubtask] — the coding
// tools WITHOUT `task`, so a subtask can't recurse into another
// delegation; (3) its goal produces just the reply string, so the tool
// result handed to the parent model is the answer text, not a TurnOutput
// blob. Its LLM rounds still record into the process budget, which
// aggregates up the subtree into the parent turn's usage roll-up.
func (e *Engine) buildSubtaskAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "task", Description: "Delegate a self-contained subtask to a fresh sub-agent that has the coding " + "tools (it cannot delegate further). Use for focused, separable work — investigate a " + "question, draft a file — so the main conversation stays uncluttered. The sub-agent starts " + "with a clean context and cannot see this conversation, so put everything it needs in the " + "prompt. It returns a single final answer; its intermediate work is not shown to the user.", Actions: []agent.Action{agent.NewAction("subtask", func(ctx context.Context, pc *core.ProcessContext, in taskInput) (string, error) {
		out, err := e.runTurn(ctx, pc, "", in.Prompt, nil, nil, accounting.Budget{})
		if err != nil {
			return "", err
		}
		return out.Reply, nil
	}, core.ActionConfig{ToolGroups: []core.ToolGroupRequirement{core.RequireToolGroup(toolport.ToolRoleSubtask)}})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "subtask answer produced"})}})
}
