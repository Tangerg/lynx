package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolset/turnctx"
)

// chatInput is the typed input to the M1 single-turn chat agent. It
// carries the user's message verbatim; future milestones extend with
// session context, tool selection hints, etc.
type chatInput struct {
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
	// reply with [ChatOutput.StoppedOnBudget] set.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost the same way MaxBudget caps
	// tokens (the lynx analog of the SDK's maxBudgetUsd). 0 means no cost
	// cap. Requires a [Config.Pricing] hook — without one cost stays 0
	// and this never trips. Either ceiling stops the turn.
	MaxCostUSD float64

	// MaxSteps caps the number of tool-call rounds (model turns) the turn may
	// run. 0 means unlimited (bounded only by the tool loop's own iteration
	// cap). When reached the action stops cleanly after the round — before the
	// next LLM call — with [ChatOutput.StoppedOnSteps] set.
	MaxSteps int
}

// ChatOutput is the typed output of one turn. Reply is the assistant's
// final text. Usage / UsageByModel / CostUSD are read back from the
// process budget — the agent framework's invocation ledger — rather
// than a hand-rolled tally: the action records each LLM round via
// [core.ProcessContext.RecordLLMInvocation], and these fields are the
// rolled-up view.
type ChatOutput struct {
	Reply string
	Usage TokenUsage

	// UsageByModel breaks Usage down per served model — the lynx analog
	// of the SDK's modelUsage. One entry for a plain single-model turn;
	// several once a turn spans models (tool rounds routed elsewhere,
	// sub-agents).
	UsageByModel []ModelUsage

	// CostUSD is the turn's total dollar cost, summed from the recorded
	// invocations. Zero unless a [Pricing] func is configured (providers
	// don't return a dollar figure on the chat path); see [Config.Pricing].
	CostUSD float64

	// StoppedOnBudget is true when the turn ended because it hit
	// [chatInput.MaxBudget] rather than the model finishing. Reply
	// holds whatever text accumulated up to the stop.
	StoppedOnBudget bool

	// StoppedOnSteps is true when the turn ended because it hit
	// [chatInput.MaxSteps] (the tool-call-round cap) rather than the model
	// finishing. Reply holds whatever text accumulated up to the stop.
	StoppedOnSteps bool
}

// buildChatAgent constructs the chat agent owned by this Engine.
// The Action's closure captures `e` so it can reach the engine's
// memory service for system-prompt composition without an extra
// parameter passed through every turn.
//
// The Action declares [toolset.ToolRoleCoding] so the runtime resolves the
// coding tool group at dispatch time; the body calls
// [core.ProcessContext.ChatWithActionTools] which composes the
// tool.NewMiddleware tool-loop on top of platform guardrails.
// The model can therefore call read / write / edit / glob / grep /
// shell freely within one turn.
//
// The body uses Stream rather than Call so each text chunk surfaces
// to [toolObserver.OnMessageDelta] as it arrives — transport
// adapters get a real streaming experience instead of one
// pre-buffered MessageDelta. Tool-call rounds still go through the
// same ToolMiddleware loop; tool events surface via the
// ToolDecorator path independently of the text-delta path.
func (e *Engine) buildChatAgent() *core.Agent {
	return agent.New("chat-agent").
		Description("single-turn LLM chat with the default coding tool set").
		Actions(agent.NewAction("chat",
			func(ctx context.Context, pc *core.ProcessContext, in chatInput) (ChatOutput, error) {
				if in.Cwd != "" {
					// Protected so it rides Blackboard.Spawn down to `task`
					// sub-agents and survives the typed-action
					// ClearBlackboard — see the tool resolver / turnctx.CwdBindingKey.
					pc.Blackboard.BindProtected(turnctx.CwdBindingKey, in.Cwd)
				}
				if in.SessionID != "" {
					// Protected for the same reasons as cwd — the read/edit
					// guards read it back via turnSession.
					pc.Blackboard.BindProtected(turnctx.SessionBindingKey, in.SessionID)
				}
				out, err := e.runChatTurn(ctx, pc, in.Provider, in.Message, in.Media, turnBudget{MaxTokens: in.MaxBudget, MaxCostUSD: in.MaxCostUSD, MaxSteps: in.MaxSteps})
				if err != nil {
					// HITL interrupt (R model): a gated tool returned an
					// agent/hitl.InterruptError that the chat tool loop
					// propagated unchanged. Park on the carried awaitable
					// (→ StatusWaiting); the client answers via a continuation
					// run. On resume the turn RE-RUNS (runChatTurn skips
					// re-adding the user message — the memory layer replays the
					// stored conversation), the model regenerates the interrupted
					// tool call, and the gate now observes the recorded verdict.
					if _, parked := hitl.HandleInterrupt(pc, err); parked {
						return ChatOutput{}, nil
					}
					return out, err
				}
				return out, nil
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor(toolset.ToolRoleCoding),
				// MaxAttempts:1 — don't let the runtime retry an LLM action.
				// Transient errors are already retried inside the model SDK;
				// permanent ones (no-access model, bad key, invalid request)
				// and ctx timeouts won't improve on retry. The default 5
				// attempts × the per-turn stall timeout is exactly what made a
				// failed run hang for minutes instead of surfacing run/closed{error}.
				QoS: core.ActionQoS{MaxAttempts: 1},
			},
		)).
		Goals(agent.GoalProducing[ChatOutput](core.Goal{
			Description: "single-turn reply produced",
		})).
		Build()
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

// buildSubtaskAgent constructs the agent behind the `task` delegation
// tool. Same chat body as the main agent, but: (1) named "task" so the
// derived tool is `task`; (2) declares [toolset.ToolRoleSubtask] — the coding
// tools WITHOUT `task`, so a subtask can't recurse into another
// delegation; (3) its goal produces just the reply string, so the tool
// result handed to the parent model is the answer text, not a ChatOutput
// blob. Its LLM rounds still record into the process budget, which
// aggregates up the subtree into the parent turn's usage roll-up.
func (e *Engine) buildSubtaskAgent() *core.Agent {
	return agent.New("task").
		Description("Delegate a self-contained subtask to a fresh sub-agent that has the coding " +
			"tools (it cannot delegate further). Use for focused, separable work — investigate a " +
			"question, draft a file — so the main conversation stays uncluttered. The sub-agent starts " +
			"with a clean context and cannot see this conversation, so put everything it needs in the " +
			"prompt. It returns a single final answer; its intermediate work is not shown to the user.").
		Actions(agent.NewAction("subtask",
			func(ctx context.Context, pc *core.ProcessContext, in taskInput) (string, error) {
				// maxBudget=0: a subtask runs without its own token cap.
				// It isn't unbounded at the turn level, though — its
				// usage records into the child budget, which aggregates
				// into the parent's subtree, so the parent turn's next
				// round-boundary budget check (which reads the subtree
				// total) stops further work once the subtask pushes the
				// parent over its budget.
				// Subtask runs against the default provider/model (no per-run
				// selection), so pass "" — invocationFrom falls back to the engine
				// default for pricing.
				out, err := e.runChatTurn(ctx, pc, "", in.Prompt, nil, turnBudget{})
				if err != nil {
					return "", err
				}
				return out.Reply, nil
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor(toolset.ToolRoleSubtask),
				QoS:        core.ActionQoS{MaxAttempts: 1}, // same rationale as the chat action
			},
		)).
		Goals(agent.GoalProducing[string](core.Goal{
			Description: "subtask answer produced",
		})).
		Build()
}

