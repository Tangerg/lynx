package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
)

// PendingSuspension identifies one unanswered external-input boundary in a
// process tree. ProcessID names the process that directly raised the boundary;
// SuspensionID is the stable identity supplied to Resume. Prompt and
// ResponseSchema are independently owned protocol values. Framework checkpoint
// state remains private to the runtime.
type PendingSuspension struct {
	ProcessID      string
	SuspensionID   string
	Prompt         json.RawMessage
	ResponseSchema json.RawMessage
}

// PendingSuspensions returns every unanswered external-input boundary in one
// idle process tree. processID must identify the tree root. Results follow
// execution order: managed tool calls keep model-call order, nested children
// occupy their parent call's position, and otherwise-independent siblings are
// ordered by process ID.
//
// The query captures the same stable complete tree as SnapshotTree, so callers
// never observe a mixture of checkpoints from different execution instants.
func (e *Engine) PendingSuspensions(ctx context.Context, processID string) ([]PendingSuspension, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.PendingSuspensions: nil Engine")
	}
	tree, err := e.SnapshotTree(ctx, processID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PendingSuspensions: %w", err)
	}
	pending, err := PendingSuspensionsIn(tree)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PendingSuspensions: %w", err)
	}
	return pending, nil
}

// PendingSuspensionsIn returns every unanswered external-input boundary in a
// caller-owned process-tree snapshot. It is the pure counterpart of
// [Engine.PendingSuspensions]: callers that also need to persist the exact
// captured tree can inspect that same value instead of taking a second capture
// and risking two observations of different execution instants.
//
// Framework checkpoint state remains private to runtime. The returned values
// contain only source identity, prompt, and response schema, each with isolated
// byte ownership.
func PendingSuspensionsIn(tree core.ProcessSnapshotTree) ([]PendingSuspension, error) {
	if err := tree.Validate(); err != nil {
		return nil, fmt.Errorf("runtime.PendingSuspensionsIn: %w", err)
	}
	if err := validateNestedSnapshotRelations(tree); err != nil {
		return nil, fmt.Errorf("runtime.PendingSuspensionsIn: %w", err)
	}
	pending, err := collectPendingSuspensions(tree)
	if err != nil {
		return nil, fmt.Errorf("runtime.PendingSuspensionsIn: %w", err)
	}
	return pending, nil
}

type pendingSuspensionCollector struct {
	snapshots map[string]core.ProcessSnapshot
	children  map[string][]string
	visited   map[string]struct{}
	pending   []PendingSuspension
}

func collectPendingSuspensions(tree core.ProcessSnapshotTree) ([]PendingSuspension, error) {
	collector := pendingSuspensionCollector{
		snapshots: make(map[string]core.ProcessSnapshot, len(tree.Snapshots)),
		children:  make(map[string][]string, len(tree.Snapshots)),
		visited:   make(map[string]struct{}, len(tree.Snapshots)),
	}
	for _, snapshot := range tree.Snapshots {
		collector.snapshots[snapshot.ID] = snapshot
		if snapshot.ParentID != "" {
			collector.children[snapshot.ParentID] = append(
				collector.children[snapshot.ParentID],
				snapshot.ID,
			)
		}
	}
	for parentID := range collector.children {
		slices.Sort(collector.children[parentID])
	}
	if err := collector.collect(tree.RootID); err != nil {
		return nil, err
	}
	if len(collector.visited) != len(tree.Snapshots) {
		return nil, fmt.Errorf(
			"%w: pending suspension traversal reached %d of %d processes",
			core.ErrInvalidSnapshot,
			len(collector.visited),
			len(tree.Snapshots),
		)
	}
	return collector.pending, nil
}

func (c *pendingSuspensionCollector) collect(processID string) error {
	if _, seen := c.visited[processID]; seen {
		return nil
	}
	snapshot, ok := c.snapshots[processID]
	if !ok {
		return fmt.Errorf("%w: process %q is missing", core.ErrInvalidSnapshot, processID)
	}
	c.visited[processID] = struct{}{}

	if snapshot.Status != core.StatusWaiting || snapshot.Suspension == nil {
		return c.collectUnvisitedChildren(processID)
	}
	checkpoint, err := decodeSuspensionCheckpoint(snapshot.Suspension.FrameworkState)
	if err != nil {
		return fmt.Errorf("process %q checkpoint: %w", processID, err)
	}
	if checkpoint == nil {
		if !snapshot.Suspension.Responded() {
			c.append(
				processID,
				snapshot.Suspension.ID,
				snapshot.Suspension.Prompt,
				snapshot.Suspension.ResponseSchema,
			)
		}
		return c.collectUnvisitedChildren(processID)
	}

	switch checkpoint.Kind {
	case suspensionCheckpointNestedChild:
		if err := c.collect(checkpoint.NestedChildren[0].ChildID); err != nil {
			return err
		}
	case suspensionCheckpointChildCanceled:
		// The canceled relation is historical correlation, not live ownership
		// and not an external input boundary.
	case suspensionCheckpointInteraction:
		if err := c.collectInteraction(processID, snapshot, checkpoint); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"%w: process %q has unknown suspension checkpoint kind %q",
			core.ErrInvalidSnapshot,
			processID,
			checkpoint.Kind,
		)
	}
	return c.collectUnvisitedChildren(processID)
}

func (c *pendingSuspensionCollector) collectInteraction(
	processID string,
	snapshot core.ProcessSnapshot,
	checkpoint *suspensionCheckpoint,
) error {
	calls, err := checkpoint.Checkpoint.ToolCalls()
	if err != nil {
		return fmt.Errorf("%w: process %q tool calls: %w", core.ErrInvalidSnapshot, processID, err)
	}
	nestedByCallID := make(map[string]*nestedChildRelation, len(checkpoint.NestedChildren))
	for _, relation := range checkpoint.NestedChildren {
		nestedByCallID[relation.ToolCallID] = relation
	}
	for index, call := range calls {
		state := checkpoint.Checkpoint.CallStates[index]
		if state.Status != toolloop.CallPaused {
			continue
		}
		if nested := nestedByCallID[call.ID]; nested != nil {
			if err := c.collect(nested.ChildID); err != nil {
				return err
			}
			continue
		}
		// A synchronous Resume may record the response without starting the
		// continuation. Only the currently exposed call can be in that state;
		// later paused calls are still unanswered.
		if index == checkpoint.Checkpoint.NextResult &&
			(snapshot.Suspension.Responded() || checkpoint.Ready) {
			continue
		}
		c.append(processID, state.Pending.ID, state.Pending.Prompt, state.Pending.ResponseSchema)
	}
	return nil
}

func (c *pendingSuspensionCollector) collectUnvisitedChildren(processID string) error {
	for _, childID := range c.children[processID] {
		if _, seen := c.visited[childID]; seen {
			continue
		}
		if err := c.collect(childID); err != nil {
			return err
		}
	}
	return nil
}

func (c *pendingSuspensionCollector) append(
	processID string,
	suspensionID string,
	prompt json.RawMessage,
	responseSchema json.RawMessage,
) {
	c.pending = append(c.pending, PendingSuspension{
		ProcessID:      processID,
		SuspensionID:   suspensionID,
		Prompt:         bytes.Clone(prompt),
		ResponseSchema: bytes.Clone(responseSchema),
	})
}

func clonePendingSuspensions(values []PendingSuspension) []PendingSuspension {
	if values == nil {
		return nil
	}
	cloned := make([]PendingSuspension, len(values))
	for index, value := range values {
		cloned[index] = PendingSuspension{
			ProcessID:      value.ProcessID,
			SuspensionID:   value.SuspensionID,
			Prompt:         bytes.Clone(value.Prompt),
			ResponseSchema: bytes.Clone(value.ResponseSchema),
		}
	}
	return cloned
}
