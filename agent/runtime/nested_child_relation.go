package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/toolcall"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	nestedChildRelationSchemaVersion uint16 = 3
	directNestedToolCallIDPrefix            = "direct:"
)

// nestedChildRelation records which child process serves which parent tool
// call. It deliberately says nothing about what that child is waiting on: the
// child's own snapshot is the single source of truth for its suspension, and a
// capture always covers the complete process tree, so a copy here would only
// be a second version of the same fact to keep in agreement.
type nestedChildRelation struct {
	SchemaVersion   uint16             `json:"schema_version"`
	ToolCallID      string             `json:"tool_call_id"`
	ChildID         string             `json:"child_id"`
	Deployment      core.DeploymentRef `json:"deployment"`
	ToolName        string             `json:"tool_name"`
	ArgumentsDigest string             `json:"arguments_digest"`
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
	cloned := *r
	return &cloned
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
		r.ToolName == other.ToolName &&
		r.ArgumentsDigest == other.ArgumentsDigest
}

func (r *nestedChildRelation) matchesCall(call chat.ToolCall) bool {
	return r != nil &&
		r.ToolCallID == call.ID &&
		r.ToolName == call.Name &&
		r.ArgumentsDigest == nestedArgumentsDigest(call.Arguments)
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
	if err := child.state.claimCheckpoint(false); err != nil {
		return nil, nil, fmt.Errorf("runtime: inspect nested child checkpoint: %w", err)
	}
	defer child.state.releaseCheckpoint()
	if child.Status() != core.StatusWaiting {
		return nil, nil, errors.New("runtime: nested child is not waiting")
	}
	suspension := child.Suspension()
	if suspension == nil || suspension.Responded() {
		return nil, nil, errors.New("runtime: nested child has no unanswered suspension")
	}
	relation := &nestedChildRelation{
		SchemaVersion:   nestedChildRelationSchemaVersion,
		ToolCallID:      toolCallID,
		ChildID:         child.ID(),
		Deployment:      child.Deployment(),
		ToolName:        toolName,
		ArgumentsDigest: nestedArgumentsDigest(arguments),
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
	if child.StartedAt().Before(parent.StartedAt()) {
		return fmt.Errorf("%w: nested child started before its parent", interaction.ErrSuspensionStale)
	}
	if child.Status() == core.StatusWaiting {
		if child.Suspension() == nil {
			return fmt.Errorf("%w: waiting nested child %q has no suspension", interaction.ErrSuspensionStale, child.ID())
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
		child.Deployment != r.Deployment {
		return fmt.Errorf("%w: nested child snapshot identity does not match relation", core.ErrInvalidSnapshot)
	}
	if child.StartedAt.Before(parent.StartedAt) {
		return fmt.Errorf("%w: nested child snapshot started before its parent", core.ErrInvalidSnapshot)
	}
	if child.Status == core.StatusWaiting {
		return ValidateResumableSnapshot(child)
	}
	if !child.Status.IsTerminal() {
		return fmt.Errorf("%w: nested child snapshot is %s", core.ErrInvalidSnapshot, child.Status)
	}
	return nil
}
