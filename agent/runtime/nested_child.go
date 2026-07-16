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
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
)

const (
	suspensionCheckpointSchemaVersion uint16 = 1
	suspensionCheckpointInteraction          = "managed_interaction"
	suspensionCheckpointNestedChild          = "nested_child"
	nestedChildRelationSchemaVersion  uint16 = 1
)

// suspensionCheckpoint is the private durable payload carried by framework
// suspensions. Managed interactions own a ToolLoop checkpoint and may also
// point at the synchronous child currently executing the pending tool call.
// Direct AgentTool calls use the nested_child form without an interaction.
type suspensionCheckpoint struct {
	SchemaVersion uint16               `json:"schema_version"`
	Kind          string               `json:"kind"`
	Owner         string               `json:"owner,omitempty"`
	Deployment    core.DeploymentRef   `json:"deployment,omitempty"`
	Checkpoint    *toolloop.Checkpoint `json:"checkpoint,omitempty"`
	NestedChild   *nestedChildRelation `json:"nested_child,omitempty"`
}

func (c *suspensionCheckpoint) validate() error {
	if c == nil || c.SchemaVersion != suspensionCheckpointSchemaVersion {
		return errors.New("runtime: invalid suspension checkpoint envelope")
	}
	switch c.Kind {
	case suspensionCheckpointInteraction:
		if c.Owner == "" || c.Checkpoint == nil {
			return errors.New("runtime: invalid managed interaction checkpoint envelope")
		}
		if err := c.Deployment.Validate(); err != nil {
			return fmt.Errorf("runtime: interaction checkpoint deployment: %w", err)
		}
		if err := c.Checkpoint.Validate(); err != nil {
			return fmt.Errorf("runtime: interaction checkpoint: %w", err)
		}
	case suspensionCheckpointNestedChild:
		if c.Owner != "" || c.Checkpoint != nil || c.Deployment != (core.DeploymentRef{}) {
			return errors.New("runtime: direct nested child checkpoint contains interaction state")
		}
	default:
		return fmt.Errorf("runtime: unknown suspension checkpoint kind %q", c.Kind)
	}
	if c.NestedChild != nil {
		if err := c.NestedChild.validate(); err != nil {
			return err
		}
	}
	if c.Kind == suspensionCheckpointNestedChild && c.NestedChild == nil {
		return errors.New("runtime: direct nested child checkpoint has no child relation")
	}
	return nil
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
		Kind string `json:"kind"`
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

func nestedRelationFromSuspension(suspension *interaction.Suspension) (*nestedChildRelation, error) {
	if suspension == nil {
		return nil, nil
	}
	checkpoint, recognized, err := decodeSuspensionCheckpoint(suspension.Payload)
	if err != nil {
		return nil, err
	}
	if !recognized || checkpoint.NestedChild == nil {
		return nil, nil
	}
	relation := checkpoint.NestedChild.clone()
	if relation.SuspensionID != suspension.ID ||
		relation.SuspensionKind != suspension.Kind ||
		!relation.SuspensionCreatedAt.Equal(suspension.CreatedAt) ||
		!bytes.Equal(relation.Prompt, suspension.Prompt) ||
		!bytes.Equal(relation.ResumeSchema, suspension.ResumeSchema) {
		return nil, fmt.Errorf("%w: nested child relation does not match parent suspension", interaction.ErrSuspensionStale)
	}
	return relation, nil
}

type nestedChildRelation struct {
	SchemaVersion       uint16                     `json:"schema_version"`
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

func (r *nestedChildRelation) matchesTool(toolName, arguments string, deployment core.DeploymentRef) bool {
	return r != nil &&
		r.ToolName == toolName &&
		r.ArgumentsDigest == nestedArgumentsDigest(arguments) &&
		r.Deployment == deployment
}

func nestedArgumentsDigest(arguments string) string {
	sum := sha256.Sum256([]byte(arguments))
	return hex.EncodeToString(sum[:])
}

func nestedRelationForChild(toolName, arguments string, child *Process) (*nestedChildRelation, *interaction.Suspension, error) {
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
	staged  *nestedChildRelation
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
	if p.nested.staged != nil && !p.nested.staged.same(relation) {
		return fmt.Errorf("%w: process %q already staged child %q", interaction.ErrSuspensionConflict, p.ID(), p.nested.staged.ChildID)
	}
	p.nested.staged = relation.clone()
	return nil
}

func (p *Process) pendingNestedChild() *nestedChildRelation {
	if p == nil {
		return nil
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	return p.nested.staged.clone()
}

func (p *Process) validateNestedSuspension(suspension interaction.Suspension) error {
	relation, err := nestedRelationFromSuspension(&suspension)
	if err != nil {
		return err
	}
	p.nested.mu.Lock()
	defer p.nested.mu.Unlock()
	switch {
	case p.nested.staged == nil && relation == nil:
		return nil
	case p.nested.staged == nil:
		return errors.New("runtime: nested suspension has no staged child")
	case relation == nil:
		return errors.New("runtime: staged nested child is missing from suspension checkpoint")
	case !p.nested.staged.same(relation):
		return fmt.Errorf("%w: staged nested child does not match suspension checkpoint", interaction.ErrSuspensionConflict)
	default:
		return nil
	}
}

func (p *Process) commitNestedSuspension() {
	if p == nil {
		return
	}
	p.nested.mu.Lock()
	p.nested.staged = nil
	p.nested.mu.Unlock()
}

func (p *Process) abortStagedNestedChild(ctx context.Context) bool {
	if p == nil {
		return false
	}
	p.nested.mu.Lock()
	relation := p.nested.staged
	p.nested.staged = nil
	p.nested.mu.Unlock()
	if relation == nil || p.engine == nil {
		return false
	}
	_ = p.engine.Kill(relation.ChildID)
	p.engine.discardProcessTree(ctx, relation.ChildID)
	return true
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
