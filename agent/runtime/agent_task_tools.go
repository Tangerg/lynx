package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"
)

// NewAgentTaskTools exposes an agent as start and result tools for background work.
//
// This is the non-blocking counterpart to [NewAgentTool]: the parent model can
// start several tasks and read their results later. The child joins the
// parent's budget subtree; cancel one outstanding task with
// [Engine.Kill], or sweep all of a turn's tasks on exit with
// [Engine.KillChildren].
//
// In is the start argument type (same schema as [NewAgentTool]);
// Out is the result type the result tool extracts via
// [core.Result]. The tools are named "<agent>_start" /
// "<agent>_result"; the start result hands the result tool a task id.
//
// Returns an error when engine is nil, agentName is empty, or the agent is
// not deployed.
func NewAgentTaskTools[In, Out any](engine *Engine, agentName string) (start tools.Tool, result tools.Tool, err error) {
	deployment, err := engine.findDeployment("NewAgentTaskTools", agentName)
	if err != nil {
		return nil, nil, err
	}
	agent := deployment.agent

	toolset := &taskToolset[In, Out]{engine: engine, deployment: deployment}

	start, err = tools.New[In, string](
		tools.Config{
			Name:        agent.Name() + "_start",
			Description: "Start " + agent.Name() + " as a background task. Returns a task_id immediately; read its result later with " + agent.Name() + "_result.",
		},
		toolset.start,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.NewAgentTaskTools: start tool: %w", err)
	}

	result, err = tools.New[taskResultInput, string](
		tools.Config{
			Name:        agent.Name() + "_result",
			Description: "Read a background " + agent.Name() + " task by task_id. Reports status running|waiting|done|failed, with the result when done.",
		},
		toolset.result,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime.NewAgentTaskTools: result tool: %w", err)
	}
	return start, result, nil
}

type taskToolset[In, Out any] struct {
	engine     *Engine
	deployment *Deployment
}

func (t *taskToolset[In, Out]) start(ctx context.Context, in In) (string, error) {
	agent := t.deployment.agent
	child, _, err := startChildDeployment(ctx, t.engine, t.deployment, in)
	if err != nil {
		return "", fmt.Errorf("background start %q: %w", agent.Name(), err)
	}
	return (taskResult{TaskID: child.ID(), Status: taskStatusRunning}).encode()
}

func (t *taskToolset[In, Out]) result(_ context.Context, input taskResultInput) (string, error) {
	agentName := t.deployment.agent.Name()
	if input.TaskID == "" {
		return "", fmt.Errorf("background result %q: task_id is required", agentName)
	}
	child, ok := t.engine.Process(input.TaskID)
	if !ok {
		return "", fmt.Errorf("background result %q: unknown task_id %q", agentName, input.TaskID)
	}
	return t.resultJSON(child)
}

// taskResultInput is the argument shape of the result tool half of
// [NewAgentTaskTools].
type taskResultInput struct {
	TaskID string `json:"task_id"`
}

// taskResult is the JSON shape the background tools hand back to the
// model. One struct documents the whole wire contract instead of
// scattering magic string keys across map[string]any literals; the
// "waiting" state is rendered separately by waitingToolResult (it
// carries the pending suspension, not a task result).
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

// resultJSON renders a background child's current state as a JSON
// tool-result string: running while the loop is live, waiting (with the
// pending suspension) when parked on HITL, done with the typed result on
// clean completion, failed with the terminal reason otherwise. Unlike
// the synchronous [agentTool.Call], a failed task is a structured result
// rather than a tool error — the model explicitly asked for the task's
// outcome, so it should be able to react instead of aborting its loop.
func (t *taskToolset[In, Out]) resultJSON(child *Process) (string, error) {
	agentName := t.deployment.agent.Name()
	switch status := child.Status(); {
	case status == core.StatusWaiting:
		return child.waitingToolResult(), nil
	case !status.IsTerminal():
		return (taskResult{TaskID: child.ID(), Status: taskStatusRunning}).encode()
	case status == core.StatusCompleted:
		out, ok := core.Result[Out](child)
		if !ok {
			var zero Out
			return "", fmt.Errorf("background result %q: completed but produced no %T", agentName, zero)
		}
		return (taskResult{TaskID: child.ID(), Status: taskStatusDone, Result: out}).encode()
	default:
		reason := ""
		if err := child.TerminalError(); err != nil {
			reason = err.Error()
		}
		return (taskResult{TaskID: child.ID(), Status: taskStatusFailed, Error: reason}).encode()
	}
}

// encode JSON-encodes a background tool reply.
func (r taskResult) encode() (string, error) {
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal task result: %w", err)
	}
	return string(encoded), nil
}
