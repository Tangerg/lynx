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
	if platform == nil {
		panic("runtime.AsChatTool: platform must not be nil")
	}
	if agentName == "" {
		panic("runtime.AsChatTool: agentName must not be empty")
	}
	agentDef, ok := platform.FindAgent(agentName)
	if !ok {
		panic(fmt.Sprintf("runtime.AsChatTool: agent %q not registered on platform", agentName))
	}
	return AsChatToolFromAgent[In, Out](platform, agentDef)
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
	if platform == nil {
		panic("runtime.AsChatToolFromAgent: platform must not be nil")
	}
	if agentDef == nil {
		panic("runtime.AsChatToolFromAgent: agent must not be nil")
	}

	var inSample In
	schema := pkgjson.MustStringDefSchemaOf(inSample)

	return &subagentTool[In, Out]{
		platform: platform,
		agent:    agentDef,
		def: chat.ToolDefinition{
			Name:        agentDef.Name,
			Description: agentDef.Description,
			InputSchema: schema,
		},
	}
}

type subagentTool[In, Out any] struct {
	platform *Platform
	agent    *core.Agent
	def      chat.ToolDefinition
}

func (t *subagentTool[In, Out]) Definition() chat.ToolDefinition { return t.def }
func (t *subagentTool[In, Out]) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }

func (t *subagentTool[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	parent := core.ProcessFrom(ctx)
	if parent == nil {
		return "", fmt.Errorf("subagent %q: no parent process in ctx (use core.WithProcess to inject one)", t.agent.Name)
	}
	parentProc, ok := t.platform.GetProcess(parent.ID())
	if !ok {
		return "", fmt.Errorf("subagent %q: parent process %q not registered on platform", t.agent.Name, parent.ID())
	}

	var in In
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &in); err != nil {
			return "", fmt.Errorf("subagent %q: parse input: %w", t.agent.Name, err)
		}
	}

	child, err := t.platform.CreateChildProcess(t.agent, parentProc, core.ProcessOptions{})
	if err != nil {
		return "", fmt.Errorf("subagent %q: create child: %w", t.agent.Name, err)
	}
	child.Blackboard().Bind(in)

	if err := t.platform.ContinueProcess(ctx, child.ID()); err != nil {
		return "", fmt.Errorf("subagent %q: run child %q: %w", t.agent.Name, child.ID(), err)
	}

	switch status := child.Status(); status {
	case core.StatusCompleted:
		// happy path — fall through
	case core.StatusWaiting:
		// Surface the child's pending request as tool-result text
		// rather than erroring. The parent's LLM sees a structured
		// description and can decide to drop the sub-agent path or
		// re-plan; the user (host) drives the child to completion via
		// Platform.ResumeProcess + ContinueProcess on the returned id.
		return waitingResultText(t.agent.Name, child), nil
	default:
		if failure := child.Failure(); failure != nil {
			return "", fmt.Errorf("subagent %q (process %q) ended in %s: %w", t.agent.Name, child.ID(), status, failure)
		}
		return "", fmt.Errorf("subagent %q (process %q) ended in %s", t.agent.Name, child.ID(), status)
	}

	out, ok := core.ResultOfType[Out](child)
	if !ok {
		var zero Out
		return "", fmt.Errorf("subagent %q completed but produced no %T", t.agent.Name, zero)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("subagent %q: marshal output: %w", t.agent.Name, err)
	}
	return string(encoded), nil
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
	if platform == nil {
		panic("runtime.AsMCPTool: platform must not be nil")
	}
	if agentName == "" {
		panic("runtime.AsMCPTool: agentName must not be empty")
	}
	agentDef, ok := platform.FindAgent(agentName)
	if !ok {
		panic(fmt.Sprintf("runtime.AsMCPTool: agent %q not registered on platform", agentName))
	}

	var inSample In
	schema := pkgjson.MustStringDefSchemaOf(inSample)

	return &mcpTool[In, Out]{
		platform: platform,
		agent:    agentDef,
		def: chat.ToolDefinition{
			Name:        agentDef.Name,
			Description: agentDef.Description,
			InputSchema: schema,
		},
	}
}

type mcpTool[In, Out any] struct {
	platform *Platform
	agent    *core.Agent
	def      chat.ToolDefinition
}

func (t *mcpTool[In, Out]) Definition() chat.ToolDefinition { return t.def }
func (t *mcpTool[In, Out]) Metadata() chat.ToolMetadata     { return chat.ToolMetadata{} }

func (t *mcpTool[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	var in In
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &in); err != nil {
			return "", fmt.Errorf("publish agent %q: parse input: %w", t.agent.Name, err)
		}
	}

	proc, err := t.platform.RunAgent(ctx, t.agent,
		map[string]any{core.DefaultBindingName: in},
		core.ProcessOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("publish agent %q: run: %w", t.agent.Name, err)
	}

	switch status := proc.Status(); status {
	case core.StatusCompleted:
		// fall through
	case core.StatusWaiting:
		return waitingResultText(t.agent.Name, proc), nil
	default:
		if failure := proc.Failure(); failure != nil {
			return "", fmt.Errorf("publish agent %q (process %q) ended in %s: %w", t.agent.Name, proc.ID(), status, failure)
		}
		return "", fmt.Errorf("publish agent %q (process %q) ended in %s", t.agent.Name, proc.ID(), status)
	}

	out, ok := core.ResultOfType[Out](proc)
	if !ok {
		var zero Out
		return "", fmt.Errorf("publish agent %q completed but produced no %T", t.agent.Name, zero)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("publish agent %q: marshal output: %w", t.agent.Name, err)
	}
	return string(encoded), nil
}
