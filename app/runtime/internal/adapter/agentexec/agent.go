package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// turnInput is the typed input to the chat turn agent: the user request only.
// Provider selection and execution limits are process policy, not planner
// state, and therefore live in ProcessOptions plus the application checkpoint.
type turnInput struct {
	Message string

	// Media carries the turn's image attachments, attached to the opening
	// user message as UserMessage.Media. Nil for a text-only turn (and for
	// `task` sub-agents, whose prompt is text).
	Media []*media.Media

	// Options carries per-run generation tuning. It deliberately does not carry
	// model selection; the process ChatProvider owns that boundary.
	Options *chat.Options
}

// TurnOutput is the typed output of one turn. Reply is the assistant's
// final text. Usage / UsageByModel / CostUSD are read back from the
// application-owned usage projection rather than a query back into Agent:
// the observer projects each managed model-response boundary, and these fields
// are the rolled-up view.
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

	// StopReason is the framework's own account of which configured bound ended
	// the interaction, carried through rather than re-labelled: the values are
	// identical and only the domain [execution.Outcome] mapping is Runtime's to
	// make. Empty on normal completion; otherwise Reply holds whatever text
	// streamed before the bound was reached.
	StopReason agent.InteractionStopReason
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
		return e.runTurn(ctx, pc, in.Message, in.Media, in.Options)
	}, core.ActionConfig{ToolGroups: []string{toolport.ToolRoleCoding}})}, Goals: []*agent.Goal{agent.NewOutputGoal[TurnOutput](core.GoalConfig{Description: "single-turn reply produced"})}})
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
		out, err := e.runTurn(ctx, pc, in.Prompt, nil, nil)
		if err != nil {
			return "", err
		}
		return out.Reply, nil
	}, core.ActionConfig{ToolGroups: []string{toolport.ToolRoleSubtask}})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "subtask answer produced"})}})
}
