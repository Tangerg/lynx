package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

// agentTool is the common wrapper behind typed agent tools and goal tools.
// Construction supplies the input decoder, process runner, and result reader.
type agentTool struct {
	engine     *Engine
	deployment *Deployment
	definition chat.ToolDefinition
	label      string
	decode     func(arguments string) (any, error)
	run        func(ctx context.Context, input any) (*Process, error)
	result     func(child *Process) (any, error)
}

func (t *agentTool) Definition() chat.ToolDefinition { return t.definition.Clone() }

func (t *agentTool) Call(ctx context.Context, arguments string) (string, error) {
	agentName := t.deployment.agent.Name()
	in, err := t.decode(arguments)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}

	parent, err := t.parentProcess(ctx)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	if parent != nil {
		relation, relationErr := nestedRelationFromSuspension(parent.Suspension())
		if relationErr != nil {
			return "", fmt.Errorf("%s %q: %w", t.label, agentName, relationErr)
		}
		if relation != nil {
			if !relation.matchesTool(t.definition.Name, arguments, t.deployment.Ref()) {
				return "", fmt.Errorf("%w: process %q is resuming nested tool %q, not %q", interaction.ErrSuspensionConflict, parent.ID(), relation.ToolName, t.definition.Name)
			}
			if suspension := parent.Suspension(); suspension == nil || !suspension.Responded() {
				return "", fmt.Errorf("%w: nested parent suspension has no response", interaction.ErrSuspensionStale)
			}
			return t.continueNestedChild(ctx, parent, relation, arguments)
		}
	}

	process, err := t.run(ctx, in)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	if parent != nil && process.ParentID() == parent.ID() && process.Status() == core.StatusWaiting {
		return "", t.suspendForNestedChild(ctx, parent, process, arguments)
	}

	defer t.discard(ctx, process)
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
	arguments string,
) (string, error) {
	child, ok := t.engine.Process(relation.ChildID)
	if !ok {
		return "", fmt.Errorf("%w: nested child process %q is missing", interaction.ErrSuspensionStale, relation.ChildID)
	}
	if err := validateNestedChildProcess(parent, child, relation); err != nil {
		return "", err
	}
	if child.Status() == core.StatusWaiting {
		suspension := child.Suspension()
		if suspension == nil || !suspension.Responded() {
			return "", fmt.Errorf("%w: nested child suspension %q has no response", interaction.ErrSuspensionStale, relation.SuspensionID)
		}
		if err := t.engine.Continue(ctx, child.ID()); err != nil {
			t.abortNestedChild(ctx, child)
			return "", fmt.Errorf("%s %q (process %q): continue nested child: %w", t.label, t.deployment.agent.Name(), child.ID(), err)
		}
	}
	if child.Status() == core.StatusWaiting {
		return "", t.suspendForNestedChild(ctx, parent, child, arguments)
	}
	if !child.Status().IsTerminal() {
		return "", fmt.Errorf("%w: nested child %q stopped in %s", interaction.ErrSuspensionStale, child.ID(), child.Status())
	}

	// The original parent suspension is now consumed. Managed interactions
	// also clear it when the ToolResult boundary commits; direct AgentTool
	// calls need this eager clear before their typed action can complete.
	parent.state.clearRespondedSuspension()
	output, err := t.encodeResult(child)
	if t.engine.ProcessStore() == nil {
		t.discard(ctx, child)
	} else {
		parent.deferNestedChildCleanup(child.ID())
	}
	return output, err
}

func (t *agentTool) suspendForNestedChild(ctx context.Context, parent, child *Process, arguments string) error {
	relation, childSuspension, err := nestedRelationForChild(t.definition.Name, arguments, child)
	if err != nil {
		t.abortNestedChild(ctx, child)
		return err
	}
	if err := parent.stageNestedChild(relation); err != nil {
		t.abortNestedChild(ctx, child)
		return err
	}
	payload, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion: suspensionCheckpointSchemaVersion,
		Kind:          suspensionCheckpointNestedChild,
		NestedChild:   relation,
	})
	if err != nil {
		parent.abortStagedNestedChild(ctx)
		return err
	}
	suspension := *childSuspension
	suspension.Payload = payload
	return &interaction.SuspendedError{Suspension: suspension}
}

func (t *agentTool) abortNestedChild(ctx context.Context, child *Process) {
	if t == nil || t.engine == nil || child == nil {
		return
	}
	if !child.Status().IsTerminal() {
		_ = t.engine.Kill(child.ID())
	}
	t.engine.discardProcessTree(ctx, child.ID())
}

// discard releases a terminal child from memory and durable storage. Waiting
// children remain registered so a host can resume them.
func (t *agentTool) discard(ctx context.Context, child *Process) {
	if t.engine == nil || child == nil || !child.Status().IsTerminal() {
		return
	}
	t.engine.discardProcessTree(ctx, child.ID())
}

func (e *Engine) discardProcessTree(ctx context.Context, processID string) {
	if e == nil || processID == "" {
		return
	}
	ctx = normalizeContext(ctx)
	for _, candidate := range e.Processes() {
		if candidate.ParentID() != processID {
			continue
		}
		if !candidate.Status().IsTerminal() {
			_ = e.Kill(candidate.ID())
		}
		e.discardProcessTree(ctx, candidate.ID())
	}
	process, ok := e.Process(processID)
	if !ok {
		if deleter, supported := e.ProcessStore().(core.SnapshotDeleter); supported {
			_ = deleter.Delete(ctx, processID)
		}
		return
	}
	if !process.Status().IsTerminal() {
		return
	}
	_ = e.Remove(processID)
	if deleter, ok := e.ProcessStore().(core.SnapshotDeleter); ok {
		_ = deleter.Delete(ctx, processID)
	}
}

// waitingToolResult renders an external/background pending suspension as a
// JSON tool result. Synchronous parent-child AgentTool calls suspend the parent
// before reaching this adapter.
//
//	{"status":"waiting", "agent":"…", "process_id":"…",
//	 "suspension_id":"…", "prompt":<payload>}
//
// Hosts can resume the child using process_id and suspension_id.
func (p *Process) waitingToolResult() string {
	agentName := p.agent().Name()
	payload := map[string]any{
		"status":     "waiting",
		"agent":      agentName,
		"process_id": p.ID(),
	}
	if suspension := p.Suspension(); suspension != nil {
		payload["suspension_id"] = suspension.ID
		payload["prompt"] = suspension.Prompt
		payload["resume_schema"] = suspension.ResumeSchema
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"status":"waiting","agent":%q,"process_id":%q}`, agentName, p.ID())
	}
	return string(encoded)
}
