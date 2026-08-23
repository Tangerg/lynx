package sessionflow

import (
	"context"
	"encoding/json"

	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/goalflow"
	"github.com/Tangerg/lynx/app2/runtime/planflow"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// snapshotItemLimit bounds cold hydration. Older transcript material remains
// available through items.list in descending pages; a mounted client never has
// to download an unbounded history before it can render or resume a Session.
const snapshotItemLimit = 200

func (service *Service) Snapshot(
	ctx context.Context,
	request protocol.GetSessionSnapshotRequest,
) (*protocol.SessionSnapshot, error) {
	material, err := service.store.ReadSessionMaterial(ctx, session.ID(request.SessionID))
	if err != nil {
		return nil, projectLookup(err)
	}

	selectedItems := snapshotItems(material.Items, material.Runs, request.IncludeDescendants)
	selectedRuns := snapshotRuns(material.Runs, selectedItems, request.IncludeDescendants)

	runs := make([]protocol.RunRef, 0, len(selectedRuns))
	includedRunIDs := make(map[string]bool, len(selectedRuns))
	for _, record := range selectedRuns {
		view, err := presentMaterialRun(record)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *view)
		includedRunIDs[record.Run.ID()] = true
	}

	items := make([]protocol.Item, 0, len(selectedItems))
	for _, record := range selectedItems {
		if !includedRunIDs[record.RunID] {
			continue
		}
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	interrupts := make([]protocol.PendingInterruptSet, 0, len(material.Interrupts))
	for _, set := range material.Interrupts {
		if request.IncludeDescendants || includedRunIDs[set.RootRunID] {
			interrupts = append(interrupts, set)
		}
	}

	var goal *protocol.Goal
	if material.Goal != nil {
		goal = presentGoal(*material.Goal)
	}
	return &protocol.SessionSnapshot{
		Items:      items,
		Runs:       runs,
		Interrupts: interrupts,
		Plan:       planflow.Present(material.Plan),
		Goal:       goal,
	}, nil
}

func snapshotItems(
	records []transcript.Record,
	runs []rundomain.Record,
	includeDescendants bool,
) []transcript.Record {
	rootOnly := make(map[string]bool, len(runs))
	for _, record := range runs {
		rootOnly[record.Run.ID()] = record.Run.ParentRunID() == ""
	}

	eligible := make([]transcript.Record, 0, len(records))
	for _, record := range records {
		if includeDescendants || rootOnly[record.RunID] {
			eligible = append(eligible, record)
		}
	}
	if len(eligible) <= snapshotItemLimit {
		return eligible
	}
	return eligible[len(eligible)-snapshotItemLimit:]
}

func snapshotRuns(
	records []rundomain.Record,
	items []transcript.Record,
	includeDescendants bool,
) []rundomain.Record {
	byID := make(map[string]rundomain.Record, len(records))
	selected := make(map[string]bool, len(items))
	for _, record := range records {
		byID[record.Run.ID()] = record
		if record.Run.Status() != rundomain.Finished &&
			(includeDescendants || record.Run.ParentRunID() == "") {
			selected[record.Run.ID()] = true
		}
	}
	for _, item := range items {
		selected[item.RunID] = true
	}
	for runID := range selected {
		for current := byID[runID]; current.Run.ID() != ""; current = byID[current.Run.ParentRunID()] {
			selected[current.Run.ID()] = true
			if current.Run.ParentRunID() == "" {
				break
			}
		}
	}

	result := make([]rundomain.Record, 0, len(selected))
	for _, record := range records {
		if !selected[record.Run.ID()] {
			continue
		}
		if !includeDescendants && record.Run.ParentRunID() != "" {
			continue
		}
		result = append(result, record)
	}
	return result
}

// Keep the Goal presenter dependency local to this use case so artifacts never
// start treating a live Goal projection as a portable persistence document.
func presentGoal(value goaldomain.Goal) *protocol.Goal {
	return goalflow.Present(value)
}
