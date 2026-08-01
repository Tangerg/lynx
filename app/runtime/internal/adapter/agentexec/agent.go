package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
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
// final text. Usage / UsageByModel / CostUSD / Steps are read back from the
// application-owned usage projection rather than a query back into Agent:
// the observer projects each managed model-response boundary, and these fields
// are the rolled-up view.
type TurnOutput struct {
	Reply string
	Usage accounting.TokenUsage
	Steps int

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
	// the interaction, carried through rather than re-labeled: the values are
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
// The Action declares [tool.GroupCoding] so the runtime resolves the
// coding tool group at dispatch time; the body calls
// [core.ProcessContext.Interact], the framework-managed interaction boundary.
// Runtime owns model/tool iteration, checkpointing, suspension, usage, and
// limits; the app supplies its prompt, streaming projection, pricing, and
// product tool policy. The model can therefore call read / write / edit / glob /
// grep / shell freely within one turn without an app-owned loop.
//
// The body uses Stream rather than Call so each text chunk surfaces
// to [executionObserver.OnMessageDelta] as it arrives — transport
// adapters get a real streaming experience instead of one pre-buffered
// MessageDelta. Tool-call rounds still go through the same tool loop; tool
// events surface via the tool-decorator path independently of the text-delta
// path.
func (e *Engine) buildTurnAgent() *core.Agent {
	chatAction := agent.NewAction(
		"chat",
		func(ctx context.Context, processCtx *core.ProcessContext, input turnInput) (TurnOutput, error) {
			return e.runTurn(ctx, processCtx, input.Message, input.Media, input.Options)
		},
		core.ActionConfig{ToolGroups: []string{tool.GroupCoding}},
	)
	replyGoal := agent.NewOutputGoal[TurnOutput](
		core.GoalConfig{Description: "single-turn reply produced"},
	)
	return agent.New(agent.AgentConfig{
		Name:        "chat-agent",
		Description: "Single-turn LLM chat with the default coding tool set.",
		Actions:     []agent.Action{chatAction},
		Goals:       []*agent.Goal{replyGoal},
	})
}

// taskInput is the argument schema the model fills to call the `task`
// tool: one self-contained subtask description. Runtime runs it in a fresh
// sub-agent with isolated context and a recursively delegating coding surface,
// then hands the sub-agent's final reply back to its parent.
type taskInput struct {
	// Description is a short (3-5 word) label for the subtask, shown in the UI
	// while it runs. Display-only: it rides in the tool-call arguments for the
	// frontend, not consumed server-side (the sub-agent works from Prompt).
	Description string `json:"description" jsonschema_description:"Short (3-5 word) description of the subtask, shown in the UI while it runs."`
	Prompt      string `json:"prompt" jsonschema_description:"The full, self-contained instructions for the sub-agent — it does not see the main conversation, so include everything it needs."`
}

// buildSubtaskAgent constructs the agent behind the `task` delegation tool.
// It shares the main agent's chat body but has three deliberate differences:
// its name derives the `task` tool name; [tool.GroupSubtask] exposes
// coding tools plus bounded recursive delegation while withholding root-only
// product tools; and its goal returns only the reply string rather than a
// TurnOutput blob. Agent Runtime's MaxChildDepth and root-owned tree budget
// bound recursion, while usage still aggregates through the process subtree.
func (e *Engine) buildSubtaskAgent() *core.Agent {
	subtaskAction := agent.NewAction(
		"subtask",
		func(ctx context.Context, processCtx *core.ProcessContext, input taskInput) (string, error) {
			output, err := e.runTurn(ctx, processCtx, input.Prompt, nil, nil)
			if err != nil {
				return "", err
			}
			return output.Reply, nil
		},
		core.ActionConfig{ToolGroups: []string{tool.GroupSubtask}},
	)
	answerGoal := agent.NewOutputGoal[string](
		core.GoalConfig{Description: "subtask answer produced"},
	)
	return agent.New(agent.AgentConfig{
		Name: "task",
		Description: "Delegate a self-contained subtask to a fresh sub-agent with coding tools and bounded task delegation. " +
			"Use it for focused, separable work so the current context stays uncluttered. " +
			"The sub-agent starts with clean context and cannot see its parent conversation, so include everything it needs in the prompt. " +
			"It returns one final answer.",
		Actions: []agent.Action{subtaskAction},
		Goals:   []*agent.Goal{answerGoal},
	})
}
