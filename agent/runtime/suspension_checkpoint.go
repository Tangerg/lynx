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
	suspensionCheckpointSchemaVersion uint16                   = 2
	suspensionCheckpointInteraction   suspensionCheckpointKind = "managed_interaction"
	suspensionCheckpointNestedChild   suspensionCheckpointKind = "nested_child"
)

// suspensionCheckpoint is the private durable payload carried by framework
// suspensions. Managed interactions own a ToolLoop checkpoint and an ordered
// subset of its paused calls may own synchronous children. Direct AgentTool
// calls use the nested_child form with exactly one relation.
type suspensionCheckpoint struct {
	SchemaVersion  uint16                   `json:"schema_version"`
	Kind           suspensionCheckpointKind `json:"kind"`
	Owner          string                   `json:"owner,omitempty"`
	Deployment     core.DeploymentRef       `json:"deployment,omitempty"`
	Checkpoint     *toolloop.Checkpoint     `json:"checkpoint,omitempty"`
	NestedChildren []*nestedChildRelation   `json:"nested_children,omitempty"`
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
		if c.Owner == "" || c.Checkpoint == nil {
			return errors.New("runtime: invalid managed interaction checkpoint envelope")
		}
		if err := c.Deployment.Validate(); err != nil {
			return fmt.Errorf("runtime: interaction checkpoint deployment: %w", err)
		}
		if _, err := validateCheckpointNestedChildren(c.Checkpoint, c.NestedChildren); err != nil {
			return err
		}
	case suspensionCheckpointNestedChild:
		if c.Owner != "" || c.Checkpoint != nil || c.Deployment != (core.DeploymentRef{}) {
			return errors.New("runtime: direct nested child checkpoint contains interaction state")
		}
		if len(c.NestedChildren) != 1 {
			return errors.New("runtime: direct nested child checkpoint must contain exactly one child relation")
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
		if !relation.matchesPending(*state.Pending) {
			return nil, fmt.Errorf("runtime: nested child relation does not match pending call %q", call.ID)
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
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, fmt.Errorf("runtime: encode suspension checkpoint: %w", err)
	}
	return payload, nil
}

// decodeSuspensionCheckpoint recognizes only the framework's discriminated
// private payload. Arbitrary application suspension payloads return
// (nil, false, nil) and remain owned by their action.
func decodeSuspensionCheckpoint(payload json.RawMessage) (*suspensionCheckpoint, bool, error) {
	var header struct {
		Kind suspensionCheckpointKind `json:"kind"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &header) != nil {
		return nil, false, nil
	}
	if header.Kind != suspensionCheckpointInteraction && header.Kind != suspensionCheckpointNestedChild {
		return nil, false, nil
	}
	var checkpoint suspensionCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return nil, true, fmt.Errorf("runtime: decode suspension checkpoint: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, true, errors.New("runtime: decode suspension checkpoint: trailing JSON value")
	}
	if err := checkpoint.validate(); err != nil {
		return nil, true, err
	}
	return &checkpoint, true, nil
}

type nestedChildCheckpoint struct {
	relations []*nestedChildRelation
	active    *nestedChildRelation
}

func (c nestedChildCheckpoint) relationForCall(toolCallID string) *nestedChildRelation {
	for _, relation := range c.relations {
		if relation.ToolCallID == toolCallID {
			return relation.clone()
		}
	}
	return nil
}

func nestedChildrenFromSuspension(suspension *interaction.Suspension) (nestedChildCheckpoint, error) {
	if suspension == nil {
		return nestedChildCheckpoint{}, nil
	}
	checkpoint, recognized, err := decodeSuspensionCheckpoint(suspension.Payload)
	if err != nil {
		return nestedChildCheckpoint{}, err
	}
	if !recognized {
		return nestedChildCheckpoint{}, nil
	}

	result := nestedChildCheckpoint{
		relations: cloneNestedChildRelations(checkpoint.NestedChildren),
	}
	switch checkpoint.Kind {
	case suspensionCheckpointNestedChild:
		result.active = result.relations[0]
	case suspensionCheckpointInteraction:
		result.active, err = validateCheckpointNestedChildren(checkpoint.Checkpoint, result.relations)
		if err != nil {
			return nestedChildCheckpoint{}, err
		}
		pending := checkpoint.Checkpoint.CallStates[checkpoint.Checkpoint.NextResult].Pending
		if suspension.ID != checkpoint.Checkpoint.ID ||
			suspension.ID != pending.ID ||
			!bytes.Equal(suspension.Prompt, pending.Prompt) ||
			!bytes.Equal(suspension.ResumeSchema, pending.ResumeSchema) {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: tool-loop checkpoint does not match parent suspension", interaction.ErrSuspensionStale)
		}
	}
	if result.active != nil && !result.active.matchesSuspension(suspension) {
		return nestedChildCheckpoint{}, fmt.Errorf("%w: active nested child does not match parent suspension", interaction.ErrSuspensionStale)
	}
	return result, nil
}
