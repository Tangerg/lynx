package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

// agentTool adapts one deployed agent to a typed child-process tool.
type agentTool struct {
	engine     *Engine
	deployment *Deployment
	definition chat.ToolDefinition
	decode     func(arguments string) (any, error)
	result     func(child *Process) (any, error)
}

func (t *agentTool) Definition() chat.ToolDefinition { return t.definition.Clone() }

// ConcurrencyKey declares AgentTool calls independent: each invocation owns an
// isolated child process. ToolLoop still commits their results in model order.
func (t *agentTool) ConcurrencyKey(string) (string, bool) { return "", true }

func (t *agentTool) Call(ctx context.Context, arguments string) (string, error) {
	agentName := t.deployment.agent.Name()
	in, err := t.decode(arguments)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}

	parent, err := t.parentProcess(ctx)
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	if parent == nil {
		return "", fmt.Errorf("agent tool %q: no parent process in ctx", agentName)
	}
	toolCallID, err := nestedToolCallID(ctx, t.definition.Name, arguments, t.deployment.Ref())
	if err != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, err)
	}
	checkpoint, relationErr := nestedChildrenFromSuspension(parent.Suspension())
	if relationErr != nil {
		return "", fmt.Errorf("agent tool %q: %w", agentName, relationErr)
	}
	relation := checkpoint.relationForCall(toolCallID)
	if relation != nil {
		if !relation.matchesToolCall(toolCallID, t.definition.Name, arguments, t.deployment.Ref()) {
			return "", fmt.Errorf("%w: process %q is resuming nested tool %q, not %q", interaction.ErrSuspensionConflict, parent.ID(), relation.ToolName, t.definition.Name)
		}
		if suspension := parent.Suspension(); suspension == nil || !suspension.Responded() {
			return "", fmt.Errorf("%w: nested parent suspension has no response", interaction.ErrSuspensionStale)
		}
		if err := parent.claimNestedChild(toolCallID, relation.ChildID); err != nil {
			return "", err
		}
		return t.continueNestedChild(ctx, parent, relation, toolCallID, arguments)
	}

	process, err := runChildDeployment(ctx, t.engine, t.deployment, in)
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
	if t == nil || t.engine == nil {
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
		if suspension == nil || !suspension.Responded() {
			return "", fmt.Errorf("%w: nested child suspension %q has no response", interaction.ErrSuspensionStale, relation.SuspensionID)
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
	parent.state.clearRespondedSuspension()
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
	relation, childSuspension, err := nestedRelationForChild(toolCallID, t.definition.Name, arguments, child)
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
	if t == nil || t.engine == nil || child == nil {
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
	if err := child.TerminalError(); err != nil {
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
