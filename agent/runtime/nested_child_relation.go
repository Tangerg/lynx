package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/toolcall"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	nestedChildRelationSchemaVersion uint16 = 2
	directNestedToolCallIDPrefix            = "direct:"
)

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

func (r *nestedChildRelation) validateProcess(parent, child *Process) error {
	if r == nil || parent == nil || child == nil {
		return errors.New("runtime: nested child relation is incomplete")
	}
	if child.ID() != r.ChildID ||
		child.ParentID() != parent.ID() ||
		child.Deployment() != r.Deployment {
		return fmt.Errorf("%w: nested child process identity does not match relation", interaction.ErrSuspensionStale)
	}
	if child.StartedAt().Before(parent.StartedAt()) ||
		r.SuspensionCreatedAt.Before(child.StartedAt()) {
		return fmt.Errorf("%w: nested child process timestamps do not match relation", interaction.ErrSuspensionStale)
	}
	if child.Status() == core.StatusWaiting {
		suspension := child.Suspension()
		if suspension == nil || suspension.ID != r.SuspensionID ||
			suspension.Kind != r.SuspensionKind ||
			!suspension.CreatedAt.Equal(r.SuspensionCreatedAt) ||
			!bytes.Equal(suspension.Prompt, r.Prompt) ||
			!bytes.Equal(suspension.ResumeSchema, r.ResumeSchema) {
			return fmt.Errorf("%w: nested child suspension does not match relation", interaction.ErrSuspensionStale)
		}
		return nil
	}
	if !child.Status().IsTerminal() {
		return fmt.Errorf("%w: nested child %q is %s", interaction.ErrSuspensionStale, child.ID(), child.Status())
	}
	return nil
}

func (r *nestedChildRelation) validateSnapshot(parent, child core.ProcessSnapshot) error {
	if err := r.validate(); err != nil {
		return err
	}
	if child.ID != r.ChildID ||
		child.ParentID != parent.ID ||
		child.Deployment != r.Deployment ||
		child.Depth != parent.Depth+1 {
		return fmt.Errorf("%w: nested child snapshot identity does not match relation", core.ErrInvalidSnapshot)
	}
	if child.StartedAt.Before(parent.StartedAt) ||
		r.SuspensionCreatedAt.Before(child.StartedAt) {
		return fmt.Errorf("%w: nested child snapshot timestamps do not match relation", core.ErrInvalidSnapshot)
	}
	if child.Status == core.StatusWaiting {
		if err := ValidateResumableSnapshot(child); err != nil {
			return err
		}
		suspension := child.Suspension
		if suspension == nil ||
			suspension.ID != r.SuspensionID ||
			suspension.Kind != r.SuspensionKind ||
			!suspension.CreatedAt.Equal(r.SuspensionCreatedAt) ||
			!bytes.Equal(suspension.Prompt, r.Prompt) ||
			!bytes.Equal(suspension.ResumeSchema, r.ResumeSchema) {
			return fmt.Errorf("%w: nested child snapshot suspension does not match relation", core.ErrInvalidSnapshot)
		}
		return nil
	}
	if !child.Status.IsTerminal() {
		return fmt.Errorf("%w: nested child snapshot is %s", core.ErrInvalidSnapshot, child.Status)
	}
	return nil
}
