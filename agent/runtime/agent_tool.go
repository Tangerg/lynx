package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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

// ConcurrencyKey declares AgentTool calls independent: each invocation owns an
// isolated child process. ToolLoop still commits their results in model order.
func (t *agentTool) ConcurrencyKey(string) (string, bool) { return "", true }

func (t *agentTool) Call(ctx context.Context, arguments string) (result string, err error) {
	agentName := t.deployment.agent.Name()
	in, err := t.decode(arguments)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}

	parent, err := t.parentProcess(ctx)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	toolCallID, err := nestedToolCallID(ctx, t.definition.Name, arguments, t.deployment.Ref())
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	if parent != nil {
		checkpoint, relationErr := nestedChildrenFromSuspension(parent.Suspension())
		if relationErr != nil {
			return "", fmt.Errorf("%s %q: %w", t.label, agentName, relationErr)
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
	}

	process, err := t.run(ctx, in)
	if err != nil {
		if process != nil {
			err = errors.Join(err, t.abortNestedChild(ctx, process))
		}
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	if parent != nil && process.ParentID() == parent.ID() && process.Status() == core.StatusWaiting {
		return "", t.suspendForNestedChild(ctx, parent, process, toolCallID, arguments)
	}

	defer func() {
		if cleanupErr := t.discard(ctx, process); cleanupErr != nil {
			result = ""
			err = errors.Join(err, cleanupErr)
		}
	}()
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
				fmt.Errorf("%s %q (process %q): continue nested child: %w", t.label, t.deployment.agent.Name(), child.ID(), err),
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
	output, err := t.encodeResult(child)
	if t.engine.ProcessStore() == nil {
		err = errors.Join(err, t.discard(ctx, child))
	} else {
		parent.deferNestedChildCleanup(child.ID())
	}
	return output, err
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
	payload, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion:  suspensionCheckpointSchemaVersion,
		Kind:           suspensionCheckpointNestedChild,
		NestedChildren: []*nestedChildRelation{relation},
	})
	if err != nil {
		parent.unstageNestedChild(toolCallID, child.ID())
		return errors.Join(err, t.abortNestedChild(ctx, child))
	}
	suspension := *childSuspension
	suspension.Payload = payload
	return &interaction.SuspendedError{Suspension: suspension}
}

func (t *agentTool) abortNestedChild(ctx context.Context, child *Process) error {
	if t == nil || t.engine == nil || child == nil {
		return nil
	}
	return t.engine.Discard(ctx, child.ID())
}

// discard releases a terminal child from memory and durable storage. Waiting
// children remain registered so a host can resume them.
func (t *agentTool) discard(ctx context.Context, child *Process) error {
	if t.engine == nil || child == nil || !child.Status().IsTerminal() {
		return nil
	}
	return t.engine.Discard(ctx, child.ID())
}

// decodeToolArguments decodes a tool argument payload into T. Empty payloads
// yield the zero value, matching calls whose fields are all optional.
func decodeToolArguments[T any](agentName, operation, arguments string) (T, error) {
	var args T
	if arguments == "" {
		return args, nil
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return args, fmt.Errorf("%s: parse input for %s: %w", agentName, operation, err)
	}
	return args, nil
}

func decodeDynamicToolArguments(agentName, operation string, inputType reflect.Type, arguments string) (any, error) {
	if inputType == nil {
		return decodeToolArguments[any](agentName, operation, arguments)
	}

	value := reflect.New(inputType)
	if arguments == "" {
		return value.Elem().Interface(), nil
	}
	if err := json.Unmarshal([]byte(arguments), value.Interface()); err != nil {
		return nil, fmt.Errorf("%s: parse input as %s for %s: %w", agentName, inputType.String(), operation, err)
	}
	return value.Elem().Interface(), nil
}

// encodeResult converts a finished child run into its tool result.
func (t *agentTool) encodeResult(child *Process) (string, error) {
	if child == nil {
		return "", errors.New("runtime.agentTool.encodeResult: child process is nil")
	}
	agentName := t.deployment.agent.Name()

	if child.Status() == core.StatusWaiting {
		return child.waitingToolResult()
	}
	if err := child.TerminalError(); err != nil {
		return "", fmt.Errorf("%s %q (process %q): %w", t.label, agentName, child.ID(), err)
	}

	out, err := t.result(child)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("%s %q: marshal output: %w", t.label, agentName, err)
	}
	return string(encoded), nil
}

// waitingProcessResult is the host-facing state of a child parked on durable
// input. Synchronous parent-child tools suspend their parent before this layer.
type waitingProcessResult struct {
	Status       taskStatus      `json:"status"`
	Agent        string          `json:"agent"`
	ProcessID    string          `json:"process_id"`
	SuspensionID string          `json:"suspension_id,omitempty"`
	Prompt       json.RawMessage `json:"prompt,omitzero"`
	ResumeSchema json.RawMessage `json:"resume_schema,omitzero"`
}

func (p *Process) waitingToolResult() (string, error) {
	if p == nil {
		return "", errors.New("runtime: waiting tool result: process is nil")
	}
	agent := p.agent()
	if agent == nil {
		return "", errors.New("runtime: waiting tool result: process has no deployed agent")
	}
	result := waitingProcessResult{
		Status:    taskStatusWaiting,
		Agent:     agent.Name(),
		ProcessID: p.ID(),
	}
	if suspension := p.Suspension(); suspension != nil {
		result.SuspensionID = suspension.ID
		result.Prompt = suspension.Prompt
		result.ResumeSchema = suspension.ResumeSchema
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("runtime: marshal waiting process result: %w", err)
	}
	return string(encoded), nil
}
