package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

// NewAgentTool wraps one exact deployment as a typed child-process tool.
// Calls require an active parent Process in ctx, start the child with clean
// working state, and aggregate the child's usage into the parent process tree.
func NewAgentTool[In, Out any](engine *Engine, deployment *Deployment) (toolcontract.Tool, error) {
	deployment, err := engine.ownedDeployment("NewAgentTool", deployment)
	if err != nil {
		return nil, err
	}
	return newAgentTool[In, Out](engine, deployment)
}

func newAgentTool[In, Out any](engine *Engine, deployment *Deployment) (toolcontract.Tool, error) {
	if deployment == nil || deployment.agent == nil {
		return nil, errors.New("runtime.newAgentTool: deployment is nil")
	}
	agent := deployment.agent

	var input In
	if any(input) == nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: input type must be concrete", agent.Name())
	}
	t := &agentTool{
		engine:     engine,
		deployment: deployment,
		result: func(child *Process) (any, error) {
			out, ok := core.Result[Out](child)
			if !ok {
				var zero Out
				return nil, fmt.Errorf("completed but produced no %T", zero)
			}
			return out, nil
		},
	}
	inner, err := toolcontract.NewFunc[In, string](
		toolcontract.FuncConfig{Name: agent.Name(), Description: agent.Description()},
		func(ctx context.Context, in In) (string, error) {
			return t.call(ctx, any(in))
		},
	)
	if err != nil {
		return nil, fmt.Errorf("runtime.newAgentTool: agent %q: build typed tool: %w", agent.Name(), err)
	}
	t.inner = inner
	return t, nil
}

// agentTool adapts one deployed agent to a typed child-process tool.
type agentTool struct {
	engine     *Engine
	deployment *Deployment
	inner      toolcontract.Tool
	result     func(child *Process) (any, error)
}

func (t *agentTool) Definition() chat.ToolDefinition {
	if t == nil || t.inner == nil {
		return chat.ToolDefinition{}
	}
	return t.inner.Definition()
}

// ConcurrencyKey declares AgentTool calls independent: each invocation owns an
// isolated child process. ToolLoop still commits their results in model order.
// AgentTool also opts into inputless continuation because Runtime, not an
// external response, may make a parked child checkpoint ready. The tool loop
// reads both declarations by capability assertion; compile-time assertions
// keep signature drift from silently changing either policy.
var (
	_ toolloop.ConcurrentTool            = (*agentTool)(nil)
	_ toolloop.InputlessContinuationTool = (*agentTool)(nil)
)

func (t *agentTool) ConcurrencyKey(string) (string, bool) { return "", true }

func (t *agentTool) CanContinueWithoutInput() bool { return true }

func (t *agentTool) Call(ctx context.Context, arguments string) (string, error) {
	if t == nil || t.inner == nil {
		return "", errors.New("runtime: AgentTool is not initialized")
	}
	return t.inner.Call(context.WithValue(ctx, agentToolArgumentsKey{}, arguments), arguments)
}

type agentToolArgumentsKey struct{}

func (t *agentTool) call(ctx context.Context, in any) (string, error) {
	agentName := t.deployment.agent.Name()
	arguments, ok := ctx.Value(agentToolArgumentsKey{}).(string)
	if !ok {
		return "", fmt.Errorf("agent tool %q: raw arguments are unavailable", agentName)
	}
	toolName := t.inner.Definition().Name

	parent, err := t.parentProcess(ctx)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	if parent == nil {
		return "", fmt.Errorf("agent tool %q: no parent process in ctx", agentName)
	}
	toolCallID, err := nestedToolCallID(ctx, toolName, arguments, t.deployment.Ref())
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	checkpoint, relationErr := nestedChildrenFromSuspension(parent.Suspension())
	if relationErr != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, relationErr)
	}
	if canceled := checkpoint.canceledForCall(toolCallID); canceled != nil {
		if !canceled.matchesToolCall(toolCallID, toolName, arguments, t.deployment.Ref()) {
			return "", fmt.Errorf("%w: process %q canceled nested tool %q, not %q", interaction.ErrSuspensionConflict, parent.ID(), canceled.ToolName, toolName)
		}
		parent.state.clearContinuableSuspension()
		return "", fmt.Errorf("agent tool %q: %w", agentName, ErrChildProcessCanceled)
	}
	relation := checkpoint.relationForCall(toolCallID)
	if relation != nil {
		if !relation.matchesToolCall(toolCallID, toolName, arguments, t.deployment.Ref()) {
			return "", fmt.Errorf("%w: process %q is resuming nested tool %q, not %q", interaction.ErrSuspensionConflict, parent.ID(), relation.ToolName, toolName)
		}
		continuable, err := suspensionContinuable(parent.Suspension())
		if err != nil {
			return "", err
		}
		if !continuable {
			return "", fmt.Errorf("%w: nested parent suspension is not continuable", interaction.ErrSuspensionStale)
		}
		if err := parent.claimNestedChild(toolCallID, relation.ChildID); err != nil {
			return "", err
		}
		return t.continueNestedChild(ctx, parent, relation, toolCallID, arguments)
	}

	process, err := runChildDeployment(ctx, t.engine, t.deployment, in, toolCallID)
	if err != nil {
		if process != nil {
			err = errors.Join(err, t.abortNestedChild(ctx, process))
		}
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	if process.ParentID() == parent.ID() && process.Status() == core.StatusWaiting {
		return "", t.suspendForNestedChild(ctx, parent, process, toolCallID, arguments)
	}

	return t.encodeResult(process)
}

func (t *agentTool) parentProcess(ctx context.Context) (*Process, error) {
	if t.engine == nil {
		return nil, nil
	}
	view := core.ProcessViewFrom(ctx)
	if view == nil {
		return nil, nil
	}
	parent, ok := t.engine.Process(view.ID())
	if !ok {
		return nil, fmt.Errorf("parent process %q is not registered on this engine", view.ID())
	}
	return parent, nil
}

func (t *agentTool) continueNestedChild(
	ctx context.Context,
	parent *Process,
	relation *nestedChildRelation,
	toolCallID string,
	arguments string,
) (string, error) {
	child, ok := t.engine.Process(relation.ChildID)
	if !ok {
		return "", fmt.Errorf("%w: nested child process %q is missing", interaction.ErrSuspensionStale, relation.ChildID)
	}
	if err := relation.validateProcess(parent, child); err != nil {
		return "", err
	}
	if child.Status() == core.StatusWaiting {
		suspension := child.Suspension()
		continuable, err := suspensionContinuable(suspension)
		if err != nil {
			return "", fmt.Errorf("runtime: inspect nested child %q continuation: %w", child.ID(), err)
		}
		if !continuable {
			return "", fmt.Errorf("%w: nested child %q is not continuable", interaction.ErrSuspensionStale, child.ID())
		}
		if err := t.engine.Continue(ctx, child.ID()); err != nil {
			cleanupErr := t.abortNestedChild(ctx, child)
			return "", errors.Join(
				fmt.Errorf("agent tool %q (process %q): continue nested child: %w", t.deployment.agent.Name(), child.ID(), err),
				cleanupErr,
			)
		}
	}
	if child.Status() == core.StatusWaiting {
		return "", t.suspendForNestedChild(ctx, parent, child, toolCallID, arguments)
	}
	if !child.Status().IsTerminal() {
		return "", fmt.Errorf("%w: nested child %q stopped in %s", interaction.ErrSuspensionStale, child.ID(), child.Status())
	}

	// The original parent suspension is now consumed. Managed interactions
	// also clear it when the ToolResult boundary commits; direct AgentTool
	// calls need this eager clear before their typed action can complete.
	parent.state.clearContinuableSuspension()
	output, resultErr := t.encodeResult(child)
	return output, resultErr
}

func (t *agentTool) suspendForNestedChild(
	ctx context.Context,
	parent *Process,
	child *Process,
	toolCallID string,
	arguments string,
) error {
	relation, childSuspension, err := nestedRelationForChild(toolCallID, t.inner.Definition().Name, arguments, child)
	if err != nil {
		return errors.Join(err, t.abortNestedChild(ctx, child))
	}
	if err := parent.stageNestedChild(relation); err != nil {
		return errors.Join(err, t.abortNestedChild(ctx, child))
	}
	frameworkState, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion:  suspensionCheckpointSchemaVersion,
		Kind:           suspensionCheckpointNestedChild,
		NestedChildren: []*nestedChildRelation{relation},
	})
	if err != nil {
		parent.unstageNestedChild(toolCallID, child.ID())
		return errors.Join(err, t.abortNestedChild(ctx, child))
	}
	suspension := *childSuspension
	suspension.FrameworkState = frameworkState
	return &interaction.SuspendedError{Suspension: suspension}
}

func (t *agentTool) abortNestedChild(ctx context.Context, child *Process) error {
	if t.engine == nil || child == nil {
		return nil
	}
	cleanupCtx := context.WithoutCancel(normalizeContext(ctx))
	return t.engine.Kill(cleanupCtx, child.ID())
}

// encodeResult converts a finished child run into its tool result.
func (t *agentTool) encodeResult(child *Process) (string, error) {
	if child == nil {
		return "", errors.New("runtime.agentTool.encodeResult: child process is nil")
	}
	agentName := t.deployment.agent.Name()

	if child.Status() == core.StatusWaiting {
		return "", fmt.Errorf("agent tool %q (process %q): waiting child was not promoted to its parent", agentName, child.ID())
	}
	if !child.Status().IsTerminal() {
		return "", fmt.Errorf("agent tool %q (process %q): child stopped in non-terminal status %s", agentName, child.ID(), child.Status())
	}
	if err := child.CompletionError(); err != nil {
		return "", fmt.Errorf("agent tool %q (process %q): %w", agentName, child.ID(), err)
	}

	out, err := t.result(child)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: marshal output: %w", agentName, err)
	}
	return string(encoded), nil
}
