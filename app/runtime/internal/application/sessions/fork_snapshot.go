package sessions

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// copyForkSnapshot projects the selected terminal boundary into the child under
// fresh global identities. A Session fork is one new aggregate, not a second
// owner for the parent's Run/Item/blob primary keys. The remap happens here in
// the use case so persistence only commits an already-coherent write set and no
// client has to synthesize the history that the model received.
func (c *Coordinator) copyForkSnapshot(
	source Snapshot,
	child session.Session,
	boundary ForkBoundary,
	steps []plan.Step,
) (Snapshot, error) {
	if len(boundary.RunIDs) == 0 {
		return Snapshot{Session: child, Messages: boundary.Messages, Plan: steps}, nil
	}
	if c.newRunID == nil || c.newItemID == nil {
		return Snapshot{}, errors.New("sessions: fork transcript identity generators are unavailable")
	}

	selectedRunIDs := make(map[string]struct{}, len(boundary.RunIDs))
	runByID := make(map[string]run.Run, len(source.Runs))
	for _, value := range source.Runs {
		runByID[value.ID()] = value
	}
	runIDs := make(map[string]string, len(boundary.RunIDs))
	for _, sourceID := range boundary.RunIDs {
		value, found := runByID[sourceID]
		if !found {
			return Snapshot{}, fmt.Errorf("sessions: fork boundary references missing run %q", sourceID)
		}
		if !value.State().IsTerminal() {
			return Snapshot{}, fmt.Errorf("sessions: fork boundary run %q is not terminal", sourceID)
		}
		selectedRunIDs[sourceID] = struct{}{}
		runIDs[sourceID] = c.newRunID()
	}

	itemIDs := make(map[string]string)
	for _, item := range source.Items {
		if _, selected := selectedRunIDs[item.RunID()]; selected {
			itemIDs[item.ID()] = c.newItemID()
		}
	}

	selectedItemIDs := make(map[string]struct{}, len(itemIDs))
	for sourceID := range itemIDs {
		selectedItemIDs[sourceID] = struct{}{}
	}
	blobIDs := make(map[toolresult.ID]toolresult.ID)
	for _, blob := range source.ToolResults {
		if _, selected := selectedItemIDs[blob.ItemID]; !selected {
			continue
		}
		if c.newToolResultID == nil {
			return Snapshot{}, errors.New("sessions: fork tool-result identity generator is unavailable")
		}
		blobIDs[blob.ID] = c.newToolResultID()
	}

	forked := Snapshot{
		Session:     child,
		Messages:    boundary.Messages,
		Runs:        make([]run.Run, 0, len(boundary.RunIDs)),
		Items:       make([]transcript.Item, 0, len(itemIDs)),
		ToolResults: make([]toolresult.Blob, 0, len(blobIDs)),
		Plan:        steps,
	}
	for _, sourceID := range boundary.RunIDs {
		value := runByID[sourceID]
		lineage := value.Lineage()
		if lineage.IsChild() {
			spawnedBy, itemFound := itemIDs[lineage.SpawnedByItemID]
			parentID, parentFound := runIDs[lineage.ParentRunID]
			rootID, rootFound := runIDs[lineage.RootRunID]
			if !itemFound || !parentFound || !rootFound {
				return Snapshot{}, fmt.Errorf("sessions: fork run %q has lineage outside the selected boundary", sourceID)
			}
			lineage = run.Lineage{
				SpawnedByItemID: spawnedBy,
				ParentRunID:     parentID,
				RootRunID:       rootID,
			}
		}
		copied, err := value.Fork(child.ID(), runIDs[sourceID], lineage)
		if err != nil {
			return Snapshot{}, fmt.Errorf("sessions: copy fork run %q: %w", sourceID, err)
		}
		forked.Runs = append(forked.Runs, copied)
	}

	for _, value := range source.Items {
		newID, selected := itemIDs[value.ID()]
		if !selected {
			continue
		}
		var offload *toolresult.Ref
		if invocation, present := value.ToolInvocation(); present && invocation.Offload != nil {
			newBlobID, found := blobIDs[invocation.Offload.ID]
			if !found {
				return Snapshot{}, fmt.Errorf("sessions: fork item %q references an unavailable tool result", value.ID())
			}
			offload = &toolresult.Ref{ID: newBlobID}
		}
		copied, err := value.Fork(child.ID(), runIDs[value.RunID()], newID, offload)
		if err != nil {
			return Snapshot{}, fmt.Errorf("sessions: copy fork item %q: %w", value.ID(), err)
		}
		forked.Items = append(forked.Items, copied)
	}

	for _, blob := range source.ToolResults {
		newBlobID, selected := blobIDs[blob.ID]
		if !selected {
			continue
		}
		blob.ID = newBlobID
		blob.SessionID = child.ID()
		blob.ItemID = itemIDs[blob.ItemID]
		forked.ToolResults = append(forked.ToolResults, blob)
	}

	normalized, err := forked.NormalizeForRestore()
	if err != nil {
		return Snapshot{}, fmt.Errorf("sessions: normalize fork snapshot: %w", err)
	}
	if err := normalized.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("sessions: validate fork snapshot: %w", err)
	}
	return normalized, nil
}
