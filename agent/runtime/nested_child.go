package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/toolcall"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

type suspensionCheckpointKind string

const (
	suspensionCheckpointSchemaVersion uint16                   = 2
	suspensionCheckpointInteraction   suspensionCheckpointKind = "managed_interaction"
	suspensionCheckpointNestedChild   suspensionCheckpointKind = "nested_child"
	nestedChildRelationSchemaVersion  uint16                   = 2
	directNestedToolCallIDPrefix                               = "direct:"
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

type nestedChildRelation struct {
	SchemaVersion       uint16                     `json:"schema_version"`
	ToolCallID          string                     `json:"tool_call_id"`
	ChildID             string                     `json:"child_id"`
	Deployment          core.DeploymentRef         `json:"deployment"`
	SuspensionID        string                     `json:"suspension_id"`
	SuspensionKind      interaction.SuspensionKind `json:"suspension_kind"`
	SuspensionCreatedAt time.Time                  `json:"suspension_created_at"`
	Prompt              json.RawMessage            `json:"prompt"`
	ResumeSchema        json.RawMessage            `json:"resume_schema"`
	ToolName            string                     `json:"tool_name"`
	ArgumentsDigest     string                     `json:"arguments_digest"`
}

func cloneNestedChildRelations(relations []*nestedChildRelation) []*nestedChildRelation {
	if relations == nil {
		return nil
	}
	cloned := make([]*nestedChildRelation, len(relations))
	for index, relation := range relations {
		cloned[index] = relation.clone()
	}
	return cloned
}

func (r *nestedChildRelation) clone() *nestedChildRelation {
	if r == nil {
		return nil
	}
	copy := *r
	copy.Prompt = bytes.Clone(r.Prompt)
	copy.ResumeSchema = bytes.Clone(r.ResumeSchema)
	return &copy
}

func (r *nestedChildRelation) validate() error {
	if r == nil || r.SchemaVersion != nestedChildRelationSchemaVersion {
		return errors.New("runtime: invalid nested child relation")
	}
	if strings.TrimSpace(r.ToolCallID) == "" || strings.TrimSpace(r.ToolCallID) != r.ToolCallID {
		return errors.New("runtime: nested child relation has invalid tool call id")
	}
	if strings.TrimSpace(r.ChildID) == "" || strings.TrimSpace(r.ChildID) != r.ChildID {
		return errors.New("runtime: nested child relation has invalid child id")
	}
	if err := r.Deployment.Validate(); err != nil {
		return fmt.Errorf("runtime: nested child deployment: %w", err)
	}
	if strings.TrimSpace(r.ToolName) == "" || strings.TrimSpace(r.ToolName) != r.ToolName {
		return errors.New("runtime: nested child relation has invalid tool name")
	}
	digest, err := hex.DecodeString(r.ArgumentsDigest)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("runtime: nested child relation has invalid arguments digest")
	}
	candidate := interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            r.SuspensionID,
		Kind:          r.SuspensionKind,
		Prompt:        r.Prompt,
		ResumeSchema:  r.ResumeSchema,
		CreatedAt:     r.SuspensionCreatedAt,
	}
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("runtime: nested child suspension: %w", err)
	}
	return nil
}

func (r *nestedChildRelation) same(other *nestedChildRelation) bool {
	if r == nil || other == nil {
		return r == other
	}
	return r.SchemaVersion == other.SchemaVersion &&
		r.ToolCallID == other.ToolCallID &&
		r.ChildID == other.ChildID &&
		r.Deployment == other.Deployment &&
		r.SuspensionID == other.SuspensionID &&
		r.SuspensionKind == other.SuspensionKind &&
		r.SuspensionCreatedAt.Equal(other.SuspensionCreatedAt) &&
		bytes.Equal(r.Prompt, other.Prompt) &&
		bytes.Equal(r.ResumeSchema, other.ResumeSchema) &&
		r.ToolName == other.ToolName &&
		r.ArgumentsDigest == other.ArgumentsDigest
}

func (r *nestedChildRelation) sameInvocation(other *nestedChildRelation) bool {
	return r != nil && other != nil &&
		r.ToolCallID == other.ToolCallID &&
		r.ChildID == other.ChildID &&
		r.Deployment == other.Deployment &&
		r.ToolName == other.ToolName &&
		r.ArgumentsDigest == other.ArgumentsDigest
}

func (r *nestedChildRelation) matchesCall(call chat.ToolCall) bool {
	return r != nil &&
		r.ToolCallID == call.ID &&
		r.ToolName == call.Name &&
		r.ArgumentsDigest == nestedArgumentsDigest(call.Arguments)
}

func (r *nestedChildRelation) matchesPending(pending toolloop.PendingCall) bool {
	return r != nil &&
		r.SuspensionID == pending.ID &&
		bytes.Equal(r.Prompt, pending.Prompt) &&
		bytes.Equal(r.ResumeSchema, pending.ResumeSchema)
}

func (r *nestedChildRelation) matchesSuspension(suspension *interaction.Suspension) bool {
	return r != nil && suspension != nil &&
		r.SuspensionID == suspension.ID &&
		r.SuspensionKind == suspension.Kind &&
		r.SuspensionCreatedAt.Equal(suspension.CreatedAt) &&
		bytes.Equal(r.Prompt, suspension.Prompt) &&
		bytes.Equal(r.ResumeSchema, suspension.ResumeSchema)
}

func (r *nestedChildRelation) matchesToolCall(
	toolCallID string,
	toolName string,
	arguments string,
	deployment core.DeploymentRef,
) bool {
	return r != nil &&
		r.ToolCallID == toolCallID &&
		r.ToolName == toolName &&
		r.ArgumentsDigest == nestedArgumentsDigest(arguments) &&
		r.Deployment == deployment
}

func nestedArgumentsDigest(arguments string) string {
	sum := sha256.Sum256([]byte(arguments))
	return hex.EncodeToString(sum[:])
}

func nestedToolCallID(
	ctx context.Context,
	toolName string,
	arguments string,
	deployment core.DeploymentRef,
) (string, error) {
	if call, ok := toolcall.FromContext(ctx); ok {
		if call.Name != toolName || call.Arguments != arguments {
			return "", errors.New("runtime: AgentTool call context does not match invocation")
		}
		return call.ID, nil
	}
	identity, err := json.Marshal(struct {
		ToolName   string             `json:"tool_name"`
		Arguments  string             `json:"arguments"`
		Deployment core.DeploymentRef `json:"deployment"`
	}{
		ToolName:   toolName,
		Arguments:  arguments,
		Deployment: deployment,
	})
	if err != nil {
		return "", fmt.Errorf("runtime: derive direct nested tool call id: %w", err)
	}
	sum := sha256.Sum256(identity)
	return directNestedToolCallIDPrefix + hex.EncodeToString(sum[:]), nil
}

func nestedRelationForChild(
	toolCallID string,
	toolName string,
	arguments string,
	child *Process,
) (*nestedChildRelation, *interaction.Suspension, error) {
	if child == nil {
		return nil, nil, errors.New("runtime: nested child is nil")
	}
	child.checkpointMu.RLock()
	defer child.checkpointMu.RUnlock()
	if child.Status() != core.StatusWaiting {
		return nil, nil, errors.New("runtime: nested child is not waiting")
	}
	suspension := child.Suspension()
	if suspension == nil || suspension.Responded() {
		return nil, nil, errors.New("runtime: nested child has no unanswered suspension")
	}
	relation := &nestedChildRelation{
		SchemaVersion:       nestedChildRelationSchemaVersion,
		ToolCallID:          toolCallID,
		ChildID:             child.ID(),
		Deployment:          child.Deployment(),
		SuspensionID:        suspension.ID,
		SuspensionKind:      suspension.Kind,
		SuspensionCreatedAt: suspension.CreatedAt,
		Prompt:              bytes.Clone(suspension.Prompt),
		ResumeSchema:        bytes.Clone(suspension.ResumeSchema),
		ToolName:            toolName,
		ArgumentsDigest:     nestedArgumentsDigest(arguments),
	}
	if err := relation.validate(); err != nil {
		return nil, nil, err
	}
	return relation, suspension, nil
}

type nestedChildState struct {
	mu      sync.Mutex
	staged  map[string]*nestedChildRelation
	pending map[string]*nestedChildRelation
	cleanup []string
}

func (p *Process) stageNestedChild(relation *nestedChildRelation) error {
	if p == nil {
		return errors.New("runtime: cannot stage nested child on nil parent")
	}
	if err := relation.validate(); err != nil {
		return err
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()

	if current := p.nested.staged[relation.ToolCallID]; current != nil {
		if current.same(relation) {
			return nil
		}
		return fmt.Errorf("%w: process %q already staged tool call %q", interaction.ErrSuspensionConflict, p.ID(), relation.ToolCallID)
	}
	if current := p.nested.pending[relation.ToolCallID]; current != nil && !current.sameInvocation(relation) {
		return fmt.Errorf("%w: process %q tool call %q changed nested child identity", interaction.ErrSuspensionConflict, p.ID(), relation.ToolCallID)
	}
	for callID, current := range p.nested.pending {
		if callID != relation.ToolCallID && current.ChildID == relation.ChildID {
			return fmt.Errorf("%w: child %q is already owned by tool call %q", interaction.ErrSuspensionConflict, relation.ChildID, callID)
		}
	}
	for callID, current := range p.nested.staged {
		if callID != relation.ToolCallID && current.ChildID == relation.ChildID {
			return fmt.Errorf("%w: child %q is already staged by tool call %q", interaction.ErrSuspensionConflict, relation.ChildID, callID)
		}
	}
	if p.nested.staged == nil {
		p.nested.staged = make(map[string]*nestedChildRelation)
	}
	p.nested.staged[relation.ToolCallID] = relation.clone()
	return nil
}

func (p *Process) currentNestedChildren() map[string]*nestedChildRelation {
	if p == nil {
		return nil
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	current := make(map[string]*nestedChildRelation, len(p.nested.pending)+len(p.nested.staged))
	for callID, relation := range p.nested.pending {
		current[callID] = relation.clone()
	}
	for callID, relation := range p.nested.staged {
		current[callID] = relation.clone()
	}
	return current
}

func (p *Process) nestedChildrenForCheckpoint(
	checkpoint *toolloop.Checkpoint,
) ([]*nestedChildRelation, *nestedChildRelation, error) {
	calls, err := checkpoint.ToolCalls()
	if err != nil {
		return nil, nil, err
	}
	current := p.currentNestedChildren()
	relations := make([]*nestedChildRelation, 0, len(current))
	for _, call := range calls {
		if relation := current[call.ID]; relation != nil {
			relations = append(relations, relation)
			delete(current, call.ID)
		}
	}
	if len(current) != 0 {
		callIDs := make([]string, 0, len(current))
		for callID := range current {
			callIDs = append(callIDs, callID)
		}
		slices.Sort(callIDs)
		return nil, nil, fmt.Errorf("%w: nested child tool call %q is absent from checkpoint", interaction.ErrSuspensionConflict, callIDs[0])
	}
	active, err := validateCheckpointNestedChildren(checkpoint, relations)
	if err != nil {
		return nil, nil, err
	}
	return relations, active, nil
}

func (p *Process) prepareNestedSuspension(suspension interaction.Suspension) (nestedChildCheckpoint, error) {
	checkpoint, err := nestedChildrenFromSuspension(&suspension)
	if err != nil {
		return nestedChildCheckpoint{}, err
	}
	current := p.currentNestedChildren()
	if len(current) != len(checkpoint.relations) {
		return nestedChildCheckpoint{}, fmt.Errorf(
			"%w: suspension has %d nested children; process has %d",
			interaction.ErrSuspensionConflict,
			len(checkpoint.relations),
			len(current),
		)
	}
	for _, relation := range checkpoint.relations {
		staged := current[relation.ToolCallID]
		if staged == nil || !staged.same(relation) {
			return nestedChildCheckpoint{}, fmt.Errorf("%w: nested child for tool call %q does not match suspension checkpoint", interaction.ErrSuspensionConflict, relation.ToolCallID)
		}
	}
	return checkpoint, nil
}

func (p *Process) commitNestedSuspension(checkpoint nestedChildCheckpoint) {
	if p == nil {
		return
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	p.nested.pending = make(map[string]*nestedChildRelation, len(checkpoint.relations))
	for _, relation := range checkpoint.relations {
		p.nested.pending[relation.ToolCallID] = relation.clone()
	}
	p.nested.staged = nil
}

func (p *Process) restoreNestedSuspension(suspension *interaction.Suspension) error {
	if p == nil {
		return nil
	}
	checkpoint, err := nestedChildrenFromSuspension(suspension)
	if err != nil {
		return err
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	p.nested.pending = make(map[string]*nestedChildRelation, len(checkpoint.relations))
	for _, relation := range checkpoint.relations {
		p.nested.pending[relation.ToolCallID] = relation.clone()
	}
	p.nested.staged = nil
	return nil
}

func (p *Process) claimNestedChild(toolCallID, childID string) error {
	if p == nil {
		return errors.New("runtime: cannot claim nested child on nil parent")
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	relation := p.nested.pending[toolCallID]
	if relation == nil || relation.ChildID != childID {
		return fmt.Errorf("%w: process %q has no pending child %q for tool call %q", interaction.ErrSuspensionStale, p.ID(), childID, toolCallID)
	}
	delete(p.nested.pending, toolCallID)
	delete(p.nested.staged, toolCallID)
	return nil
}

func (p *Process) unstageNestedChild(toolCallID, childID string) bool {
	if p == nil {
		return false
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	relation := p.nested.staged[toolCallID]
	if relation == nil || relation.ChildID != childID {
		return false
	}
	delete(p.nested.staged, toolCallID)
	return true
}

func (p *Process) abortStagedNestedChildren(ctx context.Context) int {
	if p == nil {
		return 0
	}
	p.nested.mu.Lock()
	childIDs := make([]string, 0, len(p.nested.staged))
	for _, relation := range p.nested.staged {
		childIDs = append(childIDs, relation.ChildID)
	}
	p.nested.staged = nil
	p.nested.mu.Unlock()

	slices.Sort(childIDs)
	if p.engine == nil {
		return len(childIDs)
	}
	for _, childID := range childIDs {
		_ = p.engine.Kill(childID)
		p.engine.discardProcessTree(ctx, childID)
	}
	return len(childIDs)
}

func (p *Process) deferNestedChildCleanup(childID string) {
	if p == nil || childID == "" {
		return
	}
	p.nested.mu.Lock()
	for _, current := range p.nested.cleanup {
		if current == childID {
			p.nested.mu.Unlock()
			return
		}
	}
	p.nested.cleanup = append(p.nested.cleanup, childID)
	p.nested.mu.Unlock()
}

func (p *Process) takeNestedChildCleanup() []string {
	if p == nil {
		return nil
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	cleanup := append([]string(nil), p.nested.cleanup...)
	p.nested.cleanup = nil
	return cleanup
}

func validateNestedChildProcess(parent, child *Process, relation *nestedChildRelation) error {
	if parent == nil || child == nil || relation == nil {
		return errors.New("runtime: nested child relation is incomplete")
	}
	if child.ID() != relation.ChildID ||
		child.ParentID() != parent.ID() ||
		child.Deployment() != relation.Deployment {
		return fmt.Errorf("%w: nested child process identity does not match relation", interaction.ErrSuspensionStale)
	}
	if child.StartedAt().Before(parent.StartedAt()) ||
		relation.SuspensionCreatedAt.Before(child.StartedAt()) {
		return fmt.Errorf("%w: nested child process timestamps do not match relation", interaction.ErrSuspensionStale)
	}
	if child.Status() == core.StatusWaiting {
		suspension := child.Suspension()
		if suspension == nil || suspension.ID != relation.SuspensionID ||
			suspension.Kind != relation.SuspensionKind ||
			!suspension.CreatedAt.Equal(relation.SuspensionCreatedAt) ||
			!bytes.Equal(suspension.Prompt, relation.Prompt) ||
			!bytes.Equal(suspension.ResumeSchema, relation.ResumeSchema) {
			return fmt.Errorf("%w: nested child suspension does not match relation", interaction.ErrSuspensionStale)
		}
		return nil
	}
	if !child.Status().IsTerminal() {
		return fmt.Errorf("%w: nested child %q is %s", interaction.ErrSuspensionStale, child.ID(), child.Status())
	}
	return nil
}

func validateNestedChildSnapshot(parent, child core.ProcessSnapshot, relation *nestedChildRelation) error {
	if err := relation.validate(); err != nil {
		return err
	}
	if child.ID != relation.ChildID ||
		child.ParentID != parent.ID ||
		child.Deployment != relation.Deployment ||
		child.Depth != parent.Depth+1 {
		return fmt.Errorf("%w: nested child snapshot identity does not match relation", core.ErrInvalidSnapshot)
	}
	if child.StartedAt.Before(parent.StartedAt) ||
		relation.SuspensionCreatedAt.Before(child.StartedAt) {
		return fmt.Errorf("%w: nested child snapshot timestamps do not match relation", core.ErrInvalidSnapshot)
	}
	if child.Status == core.StatusWaiting {
		if err := ValidateResumableSnapshot(child); err != nil {
			return err
		}
		suspension := child.Suspension
		if suspension == nil ||
			suspension.ID != relation.SuspensionID ||
			suspension.Kind != relation.SuspensionKind ||
			!suspension.CreatedAt.Equal(relation.SuspensionCreatedAt) ||
			!bytes.Equal(suspension.Prompt, relation.Prompt) ||
			!bytes.Equal(suspension.ResumeSchema, relation.ResumeSchema) {
			return fmt.Errorf("%w: nested child snapshot suspension does not match relation", core.ErrInvalidSnapshot)
		}
		return nil
	}
	if !child.Status.IsTerminal() {
		return fmt.Errorf("%w: nested child snapshot is %s", core.ErrInvalidSnapshot, child.Status)
	}
	return nil
}
