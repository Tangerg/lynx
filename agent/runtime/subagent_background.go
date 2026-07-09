package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// AsBackgroundChatTool exposes an agent as a PAIR of [chat.Tool]s
// implementing the background-task pattern (mirrors the SDK's
// AgentDefinition.background + stopTask): a spawn tool that launches the
// agent in the background and returns a task id immediately, and a
// collect tool that reports the task's status and — once it completes —
// its typed result.
//
// This is the non-blocking counterpart to [AsChatToolFromAgent]: instead
// of stalling its tool loop on every delegation, the parent LLM can
// spawn several long-running sub-tasks, keep reasoning, and collect each
// later. The child joins the parent's budget subtree and runs via
// [SpawnChildAsync]; cancel one outstanding task with
// [Platform.KillProcess], or sweep all of a turn's tasks on exit with
// [Platform.KillChildren].
//
// In is the spawn argument type (same schema as [AsChatToolFromAgent]);
// Out is the result type the collect tool extracts via
// [core.ResultOfType]. The tools are named "<agent>_spawn" /
// "<agent>_collect"; the spawn result hands the collect tool a task id.
//
// Returns an error when platform or agent is nil.
func AsBackgroundChatTool[In, Out any](platform *Platform, agentDef *core.Agent) (spawn chat.Tool, collect chat.Tool, err error) {
	if err = platform.validateAgent("AsBackgroundChatTool", agentDef); err != nil {
		return nil, nil, err
	}

	tools := &backgroundTools[In, Out]{platform: platform, agent: agentDef}

	spawn, err = chat.NewTool[In, string](
		chat.ToolDefinition{
			Name:        agentDef.Name + "_spawn",
			Description: "Start " + agentDef.Name + " as a background task. Returns a task_id immediately; collect its result later with " + agentDef.Name + "_collect.",
		},
		tools.spawn,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.AsBackgroundChatTool: spawn tool: %w", err)
	}

	collect, err = chat.NewTool[collectTaskInput, string](
		chat.ToolDefinition{
			Name:        agentDef.Name + "_collect",
			Description: "Collect a background " + agentDef.Name + " task by task_id. Reports status running|waiting|done|failed, with the result when done.",
		},
		tools.collect,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.AsBackgroundChatTool: collect tool: %w", err)
	}
	return spawn, collect, nil
}

type backgroundTools[In, Out any] struct {
	platform *Platform
	agent    *core.Agent
}

func (t *backgroundTools[In, Out]) spawn(ctx context.Context, in In) (string, error) {
	taskID, _, err := SpawnChildAsync(ctx, t.platform, t.agent, in)
	if err != nil {
		return "", fmt.Errorf("background spawn %q: %w", t.agent.Name, err)
	}
	return marshalTaskResult(taskResult{TaskID: taskID, Status: taskStatusRunning})
}

func (t *backgroundTools[In, Out]) collect(_ context.Context, args collectTaskInput) (string, error) {
	if args.TaskID == "" {
		return "", fmt.Errorf("background collect %q: task_id is required", t.agent.Name)
	}
	child, ok := t.platform.ProcessByID(args.TaskID)
	if !ok {
		return "", fmt.Errorf("background collect %q: unknown task_id %q", t.agent.Name, args.TaskID)
	}
	return collectResult[Out](t.agent.Name, child)
}

// collectTaskInput is the argument shape of the collect tool half of
// [AsBackgroundChatTool].
type collectTaskInput struct {
	TaskID string `json:"task_id"`
}

// taskResult is the JSON shape the background tools hand back to the
// model. One struct documents the whole wire contract instead of
// scattering magic string keys across map[string]any literals; the
// "waiting" state is rendered separately by waitingResultText (it
// carries the pending awaitable, not a task result).
type taskResult struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

const (
	taskStatusRunning = "running"
	taskStatusDone    = "done"
	taskStatusFailed  = "failed"
)

// collectResult renders a background child's current state as a JSON
// tool-result string: running while the loop is live, waiting (with the
// pending awaitable) when parked on HITL, done with the typed result on
// clean completion, failed with the terminal reason otherwise. Unlike
// the synchronous [agentTool.Call], a failed task is a structured result
// rather than a tool error — the model explicitly asked for the task's
// outcome, so it should be able to react instead of aborting its loop.
func collectResult[Out any](agentName string, child *AgentProcess) (string, error) {
	switch status := child.Status(); {
	case status == core.StatusWaiting:
		return waitingResultText(agentName, child), nil
	case !status.IsTerminal():
		return marshalTaskResult(taskResult{TaskID: child.ID(), Status: taskStatusRunning})
	case status == core.StatusCompleted:
		out, ok := core.ResultOfType[Out](child)
		if !ok {
			var zero Out
			return "", fmt.Errorf("background collect %q: completed but produced no %T", agentName, zero)
		}
		return marshalTaskResult(taskResult{TaskID: child.ID(), Status: taskStatusDone, Result: out})
	default:
		reason := ""
		if err := child.TerminalError(); err != nil {
			reason = err.Error()
		}
		return marshalTaskResult(taskResult{TaskID: child.ID(), Status: taskStatusFailed, Error: reason})
	}
}

// marshalTaskResult JSON-encodes a background tool reply.
func marshalTaskResult(r taskResult) (string, error) {
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal task result: %w", err)
	}
	return string(encoded), nil
}
