package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// AsChatTool wraps a deployed agent as a [chat.CallableTool] the
// parent's LLM can invoke as just-another-tool. This is the
// "supervisor" pattern: a parent agent's body uses
// [core.ProcessContext.ChatWithActionTools] to ask the LLM, the LLM
// picks one of several sub-agent tools, the tool runs the sub-agent
// synchronously inside this process, and the result feeds back into
// the LLM's tool loop.
//
// In is the type the sub-agent's first action consumes; the LLM's
// tool-call argument blob is JSON-decoded into In and bound onto the
// child's blackboard via dual-binding.
// Out is the type the sub-agent produces; AsChatTool extracts it via
// [core.ResultOfType] and JSON-encodes it as the tool result.
//
// The child process inherits the parent's blackboard via
// [Platform.CreateChildProcess], so artifacts the parent has staged
// are visible to the child. Budget aggregation is automatic — the
// parent's [core.Process.Usage] sums the entire delegation tree.
//
// Panics on construction when platform is nil or agentName isn't
// registered: programming errors should fail at boot, not on the
// first LLM tool call.
//
// Example:
//
//	tools := []chat.Tool{
//	    runtime.AsChatTool[Topic, Brief](platform, "research-agent"),
//	    runtime.AsChatTool[Brief, BlogPost](platform, "writer-agent"),
//	}
//	req, _ := pc.Chat().WithTools(tools...).Call().Text(ctx)
func AsChatTool[In, Out any](platform *Platform, agentName string) chat.CallableTool {
	agentDef := mustFindAgent("AsChatTool", platform, agentName)
	return newAgentTool[In, Out]("subagent", platform, agentDef, runAsChild[In])
}

// AsChatToolFromAgent is the [AsChatTool] sibling that takes a
// *core.Agent directly instead of looking up by name on the
// platform. Use when the caller already holds the agent struct (e.g.
// constructed via [agent.New(...).Build()] but not yet deployed) and
// wants to skip the registry lookup, or when the agent is in flight
// across registration races. The agent need NOT be deployed on
// platform — child processes spawned from it land on platform.procs
// the same way [AsChatTool] does.
//
// Panics on nil platform or nil agent.
func AsChatToolFromAgent[In, Out any](platform *Platform, agentDef *core.Agent) chat.CallableTool {
	mustValidate("AsChatToolFromAgent", platform, agentDef)
	return newAgentTool[In, Out]("subagent", platform, agentDef, runAsChild[In])
}

// AsMCPTool is the top-level companion to [AsChatTool]: it wraps a
// deployed agent as a [chat.CallableTool] that an external MCP host
// (Claude Desktop, Cursor, …) can drive. The returned tool spins up
// a *fresh* process per call (no parent context required) so it's
// suitable for the "agent.action(input) → output" RPC pattern.
//
// Compose with [github.com/Tangerg/lynx/mcp].RegisterTools:
//
//	server := sdkmcp.NewServer(impl, nil)
//	mcp.RegisterTools(server, runtime.AsMCPTool[Topic, Brief](platform, "BriefingAgent"))
//
// embabel's `PerGoalMcpExportToolCallbackPublisher` does the same
// in batch over an agent's goals; lynx's typical "one goal per
// agent" shape makes the per-call form ergonomic enough that we
// don't ship a separate batch helper.
//
// Suspended (HITL) runs surface the same JSON "status: waiting"
// payload [AsChatTool] uses, so an MCP host can decide to drive the
// process via [Platform.ResumeProcess] out of band.
func AsMCPTool[In, Out any](platform *Platform, agentName string) chat.CallableTool {
	agentDef := mustFindAgent("AsMCPTool", platform, agentName)
	return newAgentTool[In, Out]("publish agent", platform, agentDef, runAsTopLevel[In])
}

// runProcessFunc starts (or resumes) an [*AgentProcess] for one
// agent-as-tool invocation. The strategy varies by call site:
//
//   - [runAsChild]    — supervisor flow: requires parent process in
//     ctx, spawns a child via [Platform.CreateChildProcess], binds
//     the typed In, then drives the loop via
//     [Platform.ContinueProcess].
//   - [runAsTopLevel] — MCP-publish flow: no parent expected,
//     [Platform.RunAgent] starts a fresh process with In bound under
//     [core.DefaultBindingName].
type runProcessFunc[In any] func(ctx context.Context, platform *Platform, agentDef *core.Agent, in In) (*AgentProcess, error)

// agentTool is the common chat-tool wrapper for "run an agent and
// surface its typed output as a JSON tool result". The runProc
// strategy is the only thing that varies between supervisor /
// MCP-publish callsites.
type agentTool[In, Out any] struct {
	label    string // surfaces in error messages — "subagent" / "publish agent"
	platform *Platform
	agent    *core.Agent
	def      chat.ToolDefinition
	runProc  runProcessFunc[In]
}

func (t *agentTool[In, Out]) Definition() chat.ToolDefinition { return t.def }
func (t *agentTool[In, Out]) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }

func (t *agentTool[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	var in In
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &in); err != nil {
			return "", fmt.Errorf("%s %q: parse input: %w", t.label, t.agent.Name, err)
		}
	}

	proc, err := t.runProc(ctx, t.platform, t.agent, in)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, t.agent.Name, err)
	}

	switch status := proc.Status(); status {
	case core.StatusCompleted:
		// fall through to result extraction
	case core.StatusWaiting:
		return waitingResultText(t.agent.Name, proc), nil
	default:
		if failure := proc.Failure(); failure != nil {
			return "", fmt.Errorf("%s %q (process %q) ended in %s: %w", t.label, t.agent.Name, proc.ID(), status, failure)
		}
		return "", fmt.Errorf("%s %q (process %q) ended in %s", t.label, t.agent.Name, proc.ID(), status)
	}

	out, ok := core.ResultOfType[Out](proc)
	if !ok {
		var zero Out
		return "", fmt.Errorf("%s %q completed but produced no %T", t.label, t.agent.Name, zero)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("%s %q: marshal output: %w", t.label, t.agent.Name, err)
	}
	return string(encoded), nil
}

// newAgentTool packages the strategy into a [chat.CallableTool]. The
// JSON schema for In is derived once at construction time so the
// LLM sees a stable shape across calls.
func newAgentTool[In, Out any](
	label string,
	platform *Platform,
	agentDef *core.Agent,
	runProc runProcessFunc[In],
) chat.CallableTool {
	var inSample In
	schema := pkgjson.MustStringDefSchemaOf(inSample)

	return &agentTool[In, Out]{
		label:    label,
		platform: platform,
		agent:    agentDef,
		def: chat.ToolDefinition{
			Name:        agentDef.Name,
			Description: agentDef.Description,
			InputSchema: schema,
		},
		runProc: runProc,
	}
}

// mustFindAgent looks the agent up by name and panics — at construction
// time, not on the first LLM tool call — when the registration is
// missing. Shared between AsChatTool / AsMCPTool.
func mustFindAgent(label string, platform *Platform, name string) *core.Agent {
	if platform == nil {
		panic(fmt.Sprintf("runtime.%s: platform must not be nil", label))
	}
	if name == "" {
		panic(fmt.Sprintf("runtime.%s: agentName must not be empty", label))
	}
	agentDef, ok := platform.FindAgent(name)
	if !ok {
		panic(fmt.Sprintf("runtime.%s: agent %q not registered on platform", label, name))
	}
	return agentDef
}

// mustValidate is the [AsChatToolFromAgent] companion: same nil
// checks as [mustFindAgent] minus the registry lookup.
func mustValidate(label string, platform *Platform, agentDef *core.Agent) {
	if platform == nil {
		panic(fmt.Sprintf("runtime.%s: platform must not be nil", label))
	}
	if agentDef == nil {
		panic(fmt.Sprintf("runtime.%s: agent must not be nil", label))
	}
}

// runAsChild is the supervisor strategy: requires a parent process in
// ctx, spawns a child via [Platform.CreateChildProcess], binds the
// typed input on the child blackboard, and drives the loop via
// [Platform.ContinueProcess].
func runAsChild[In any](ctx context.Context, platform *Platform, agentDef *core.Agent, in In) (*AgentProcess, error) {
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return nil, fmt.Errorf("no parent process in ctx (use core.WithProcess to inject one)")
	}
	parentProc, ok := platform.GetProcess(parent.ID())
	if !ok {
		return nil, fmt.Errorf("parent process %q not registered on platform", parent.ID())
	}

	child, err := platform.CreateChildProcess(agentDef, parentProc, core.ProcessOptions{})
	if err != nil {
		return nil, fmt.Errorf("create child: %w", err)
	}
	child.Blackboard().Bind(in)

	if err := platform.ContinueProcess(ctx, child.ID()); err != nil {
		return nil, fmt.Errorf("run child %q: %w", child.ID(), err)
	}
	return child, nil
}

// runAsTopLevel is the MCP-publish strategy: a fresh process per
// call, the typed input flows in as the [core.DefaultBindingName].
func runAsTopLevel[In any](ctx context.Context, platform *Platform, agentDef *core.Agent, in In) (*AgentProcess, error) {
	proc, err := platform.RunAgent(ctx, agentDef,
		map[string]any{core.DefaultBindingName: in},
		core.ProcessOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	return proc, nil
}

// waitingResultText renders a JSON description of a sub-agent's
// pending await as a tool-result string. The parent LLM sees:
//
//	{"status":"waiting", "agent":"…", "processId":"…",
//	 "awaitableId":"…", "prompt":<payload>}
//
// "prompt" is whatever [core.Awaitable.PromptAny] returns — typically
// the human-facing payload of a [hitl.TypedRequest]. Hosts can drive
// the child to completion via [Platform.ResumeProcess] +
// [Platform.ContinueProcess] using the returned processId; the
// returned text is informational, suited for the LLM's next-turn
// reasoning.
func waitingResultText(agentName string, child *AgentProcess) string {
	payload := map[string]any{
		"status":    "waiting",
		"agent":     agentName,
		"processId": child.ID(),
	}
	if a := child.PendingAwaitable(); a != nil {
		payload["awaitableId"] = a.ID()
		payload["prompt"] = a.PromptAny()
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// Fallback to a plain sentence if marshal somehow fails — keeps
		// the LLM-visible result useful even in degenerate cases.
		return fmt.Sprintf(`{"status":"waiting","agent":%q,"processId":%q}`, agentName, child.ID())
	}
	return string(encoded)
}
