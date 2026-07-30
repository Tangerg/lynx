package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
)

type suspensionCheckpointKind string

const (
	suspensionCheckpointSchemaVersion uint16                   = 3
	suspensionCheckpointInteraction   suspensionCheckpointKind = "managed_interaction"
	suspensionCheckpointNestedChild   suspensionCheckpointKind = "nested_child"
	suspensionCheckpointChildCanceled suspensionCheckpointKind = "nested_child_canceled"
)

// suspensionCheckpoint is the private continuation state carried by framework
// suspensions. Managed interactions own a ToolLoop checkpoint and an ordered
// subset of its paused calls may own synchronous children. Direct AgentTool
// calls use the nested_child form with exactly one relation.
type suspensionCheckpoint struct {
	SchemaVersion  uint16                   `json:"schema_version"`
	Kind           suspensionCheckpointKind `json:"kind"`
	Owner          string                   `json:"owner,omitempty"`
	Deployment     core.DeploymentRef       `json:"deployment,omitzero"`
	Checkpoint     *toolloop.Checkpoint     `json:"checkpoint,omitempty"`
	NestedChildren []*nestedChildRelation   `json:"nested_children,omitempty"`
	Ready          bool                     `json:"ready,omitempty"`
	CanceledChild  *nestedChildRelation     `json:"canceled_child,omitempty"`
}

func (c *suspensionCheckpoint) validate() error {
	if c == nil || c.SchemaVersion != suspensionCheckpointSchemaVersion {
		return errors.New("runtime: invalid suspension checkpoint envelope")
	}
	if err := validateNestedChildRelations(c.NestedChildren); err != nil {
		return err
	}
	switch c.Kind {
	case suspensionCheckpointInteraction:
		if c.Owner == "" || c.Checkpoint == nil || c.CanceledChild != nil {
			return errors.New("runtime: invalid managed interaction checkpoint envelope")
		}
		if err := c.Deployment.Validate(); err != nil {
			return fmt.Errorf("runtime: interaction checkpoint deployment: %w", err)
		}
		active, err := validateCheckpointNestedChildren(c.Checkpoint, c.NestedChildren)
		if err != nil {
			return err
		}
		if c.Ready && active == nil {
			return errors.New("runtime: ready managed interaction has no active nested child")
		}
	case suspensionCheckpointNestedChild:
		if c.Owner != "" || c.Checkpoint != nil || c.Deployment != (core.DeploymentRef{}) ||
			c.CanceledChild != nil {
			return errors.New("runtime: direct nested child checkpoint contains interaction state")
		}
		if len(c.NestedChildren) != 1 {
			return errors.New("runtime: direct nested child checkpoint must contain exactly one child relation")
		}
	case suspensionCheckpointChildCanceled:
		if c.Owner != "" || c.Checkpoint != nil || c.Deployment != (core.DeploymentRef{}) ||
			len(c.NestedChildren) != 0 || c.Ready {
			return errors.New("runtime: canceled nested child checkpoint contains live continuation state")
		}
		if err := c.CanceledChild.validate(); err != nil {
			return fmt.Errorf("runtime: canceled nested child: %w", err)
		}
	default:
		return fmt.Errorf("runtime: unknown suspension checkpoint kind %q", c.Kind)
	}
	return nil
}

func validateNestedChildRelations(relations []*nestedChildRelation) error {
	callIDs := make(map[string]struct{}, len(relations))
	childIDs := make(map[string]struct{}, len(relations))
	for index, relation := range relations {
		if err := relation.validate(); err != nil {
			return fmt.Errorf("runtime: nested_children[%d]: %w", index, err)
		}
		if _, duplicate := callIDs[relation.ToolCallID]; duplicate {
			return fmt.Errorf("runtime: duplicate nested child tool call %q", relation.ToolCallID)
		}
		callIDs[relation.ToolCallID] = struct{}{}
		if _, duplicate := childIDs[relation.ChildID]; duplicate {
			return fmt.Errorf("runtime: duplicate nested child process %q", relation.ChildID)
		}
		childIDs[relation.ChildID] = struct{}{}
	}
	return nil
}

// validateCheckpointNestedChildren verifies that relations are an ordered
// subset of paused ToolLoop calls and returns the relation for the currently
// exposed pause, when that active call is an AgentTool.
func validateCheckpointNestedChildren(
	checkpoint *toolloop.Checkpoint,
	relations []*nestedChildRelation,
) (*nestedChildRelation, error) {
	calls, err := checkpoint.ToolCalls()
	if err != nil {
		return nil, fmt.Errorf("runtime: interaction checkpoint: %w", err)
	}
	byCallID := make(map[string]*nestedChildRelation, len(relations))
	for _, relation := range relations {
		byCallID[relation.ToolCallID] = relation
	}

	relationIndex := 0
	var active *nestedChildRelation
	for callIndex, call := range calls {
		relation := byCallID[call.ID]
		if relation == nil {
			continue
		}
		if relationIndex >= len(relations) || relations[relationIndex].ToolCallID != call.ID {
			return nil, errors.New("runtime: nested child relations are not in tool-call order")
		}
		state := checkpoint.CallStates[callIndex]
		if state.Status != toolloop.CallPaused || state.Pending == nil {
			return nil, fmt.Errorf("runtime: nested child call %q is not paused", call.ID)
		}
		if !relation.matchesCall(call) {
			return nil, fmt.Errorf("runtime: nested child relation does not match tool call %q", call.ID)
		}
		if callIndex == checkpoint.NextResult {
			active = relation
		}
		relationIndex++
	}
	if relationIndex != len(relations) {
		return nil, errors.New("runtime: nested child relation references an unknown tool call")
	}
	return active, nil
}

func encodeSuspensionCheckpoint(checkpoint suspensionCheckpoint) (json.RawMessage, error) {
	if err := checkpoint.validate(); err != nil {
		return nil, err
	}
	state, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("runtime: encode suspension checkpoint: %w", err)
	}
	return state, nil
}

// decodeSuspensionCheckpoint decodes the framework-owned continuation state.
// Every failure below is a statement about captured state, so this is where
// that classification is applied — once, rather than on each leaf check, so a
// validation error added later cannot reach a caller unmarked.
func decodeSuspensionCheckpoint(state json.RawMessage) (*suspensionCheckpoint, error) {
	if len(state) == 0 {
		return nil, nil
	}
	checkpoint, err := parseSuspensionCheckpoint(state)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", core.ErrInvalidSnapshot, err)
	}
	return checkpoint, nil
}

func parseSuspensionCheckpoint(state json.RawMessage) (*suspensionCheckpoint, error) {
	var checkpoint suspensionCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(state))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return nil, fmt.Errorf("runtime: decode suspension checkpoint: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("runtime: decode suspension checkpoint: trailing JSON value")
	}
	if err := checkpoint.validate(); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

type nestedChildCheckpoint struct {
	relations []*nestedChildRelation
	active    *nestedChildRelation
	canceled  *nestedChildRelation
	ready     bool
}

func (c nestedChildCheckpoint) relationForCall(toolCallID string) *nestedChildRelation {
	for _, relation := range c.relations {
		if relation.ToolCallID == toolCallID {
			return relation.clone()
		}
	}
	return nil
}

func (c nestedChildCheckpoint) canceledForCall(toolCallID string) *nestedChildRelation {
	if c.canceled != nil && c.canceled.ToolCallID == toolCallID {
		return c.canceled.clone()
	}
	return nil
}

func nestedChildrenFromSuspension(suspension *interaction.Suspension) (nestedChildCheckpoint, error) {
	if suspension == nil {
		return nestedChildCheckpoint{}, nil
	}
	checkpoint, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return nestedChildCheckpoint{}, err
	}
	if checkpoint == nil {
		return nestedChildCheckpoint{}, nil
	}
	if checkpoint.Ready && suspension.Responded() {
		return nestedChildCheckpoint{}, fmt.Errorf("%w: framework-ready suspension carries an external response", interaction.ErrSuspensionStale)
	}

	result := nestedChildCheckpoint{
		relations: cloneNestedChildRelations(checkpoint.NestedChildren),
		canceled:  checkpoint.CanceledChild.clone(),
		ready:     checkpoint.Ready,
	}
	switch checkpoint.Kind {
	case suspensionCheckpointNestedChild:
		result.active = result.relations[0]
	case suspensionCheckpointChildCanceled:
		return result, nil
	case suspensionCheckpointInteraction:
		result.active, err = validateCheckpointNestedChildren(checkpoint.Checkpoint, result.relations)
		if err != nil {
			return nestedChildCheckpoint{}, err
		}
		pending, awaitingInput, err := checkpoint.Checkpoint.AwaitingInput()
		if err != nil {
			return nestedChildCheckpoint{}, fmt.Errorf("runtime: interaction checkpoint: %w", err)
		}
		if suspension.ID != checkpoint.Checkpoint.ID {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: tool-loop checkpoint does not match parent suspension", interaction.ErrSuspensionStale)
		}
		if awaitingInput &&
			(suspension.ID != pending.ID ||
				!bytes.Equal(suspension.Prompt, pending.Prompt) ||
				!bytes.Equal(suspension.ResumeSchema, pending.ResumeSchema)) {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: tool-loop checkpoint does not match parent suspension", interaction.ErrSuspensionStale)
		}
		if !awaitingInput && suspension.Responded() {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: ready tool-loop checkpoint carries an external response", interaction.ErrSuspensionStale)
		}
	}
	return result, nil
}

// suspensionContinuable reports whether runtime-owned state can re-enter this
// suspension without accepting a new external response. A normal answered
// suspension remains continuable; framework readiness covers a host-settled
// tool result and a live nested child whose own checkpoint is ready.
func suspensionContinuable(suspension *interaction.Suspension) (bool, error) {
	if suspension == nil {
		return false, nil
	}
	if suspension.Responded() {
		return true, nil
	}
	checkpoint, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return false, err
	}
	if checkpoint == nil {
		return false, nil
	}
	switch checkpoint.Kind {
	case suspensionCheckpointChildCanceled:
		return true, nil
	case suspensionCheckpointNestedChild:
		return checkpoint.Ready, nil
	case suspensionCheckpointInteraction:
		_, awaitingInput, err := checkpoint.Checkpoint.AwaitingInput()
		if err != nil {
			return false, fmt.Errorf("runtime: interaction checkpoint: %w", err)
		}
		return !awaitingInput || checkpoint.Ready, nil
	default:
		return false, fmt.Errorf("runtime: unknown suspension checkpoint kind %q", checkpoint.Kind)
	}
}
