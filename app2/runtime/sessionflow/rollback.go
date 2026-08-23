package sessionflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (service *Service) Rollback(
	ctx context.Context,
	request protocol.RollbackSessionRequest,
) (*RollbackResult, error) {
	material, err := service.store.ReadSessionMaterial(ctx, session.ID(request.SessionID))
	if err != nil {
		return nil, projectLookup(err)
	}
	for _, record := range material.Runs {
		if record.Run.Status() != rundomain.Finished {
			return nil, protocol.ErrSessionBusy
		}
	}

	restoreType, err := normalizeRestoreType(request)
	if err != nil {
		return nil, err
	}
	roots, keepIndex, err := rollbackBoundary(material.Runs, request.ToRunID)
	if err != nil {
		return nil, err
	}

	if restoreType == protocol.RestoreFiles || restoreType == protocol.RestoreBoth {
		if err := service.checkpoints.Restore(
			ctx,
			request.SessionID,
			material.Session.Workspace().Path(),
			request.ToRunID,
		); err != nil {
			return nil, fmt.Errorf("%w: %v", protocol.ErrCheckpointUnavailable, err)
		}
	}
	if restoreType == protocol.RestoreFiles {
		resolved, err := service.workspaces.Resolve(ctx, material.Session.Workspace().Path())
		if err != nil {
			return nil, err
		}
		return &RollbackResult{Response: &protocol.RollbackSessionResponse{
			Session:     present(material.Session, resolved, session.StatusIdle),
			DroppedRuns: []protocol.DroppedRun{},
		}}, nil
	}

	droppedRoots := roots[keepIndex+1:]
	if len(droppedRoots) == 0 {
		resolved, err := service.workspaces.Resolve(ctx, material.Session.Workspace().Path())
		if err != nil {
			return nil, err
		}
		return &RollbackResult{Response: &protocol.RollbackSessionResponse{
			Session:     present(material.Session, resolved, session.StatusIdle),
			DroppedRuns: []protocol.DroppedRun{},
		}}, nil
	}

	dropped, dropRootIDs, dropRunIDs, err := describeDroppedRuns(material, droppedRoots)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	replacement, err := rollbackPlan(material, roots, keepIndex, now)
	if err != nil {
		return nil, err
	}
	updated, err := service.store.RollbackSessionHistory(ctx, RollbackWrite{
		SessionID:            session.ID(request.SessionID),
		DropRootRunIDs:       dropRootIDs,
		Plan:                 replacement,
		ExpectedPlanRevision: material.Plan.Revision(),
		Now:                  now,
	})
	if errors.Is(err, plandomain.ErrVersionConflict) {
		return nil, protocol.ErrRevisionConflict
	}
	if err != nil {
		return nil, err
	}
	if err := service.checkpoints.DropRuns(ctx, request.SessionID, dropRunIDs); err != nil {
		return nil, fmt.Errorf("sessionflow: prune dropped checkpoints: %w", err)
	}

	resolved, err := service.workspaces.Resolve(ctx, updated.Workspace().Path())
	if err != nil {
		return nil, err
	}
	return &RollbackResult{
		Response: &protocol.RollbackSessionResponse{
			Session:     present(updated, resolved, session.StatusIdle),
			DroppedRuns: dropped,
		},
		PlanChanged:    replacement != nil,
		HistoryChanged: true,
	}, nil
}

func normalizeRestoreType(request protocol.RollbackSessionRequest) (protocol.RestoreType, error) {
	restoreType := request.RestoreType
	if restoreType == "" {
		restoreType = protocol.RestoreHistory
	}
	switch restoreType {
	case protocol.RestoreHistory:
		return restoreType, nil
	case protocol.RestoreFiles, protocol.RestoreBoth:
		if request.ToRunID == "" {
			return "", fmt.Errorf("%w: file restore requires toRunId", protocol.ErrInvalidParams)
		}
		return restoreType, nil
	default:
		return "", fmt.Errorf("%w: invalid restoreType", protocol.ErrInvalidParams)
	}
}

func rollbackBoundary(
	records []rundomain.Record,
	toRunID string,
) ([]rundomain.Record, int, error) {
	roots := make([]rundomain.Record, 0)
	for _, record := range records {
		if record.Run.ParentRunID() == "" {
			roots = append(roots, record)
		}
	}
	slices.SortFunc(roots, compareRunRecords)
	keepIndex := -1
	if toRunID != "" {
		keepIndex = slices.IndexFunc(roots, func(record rundomain.Record) bool {
			return record.Run.ID() == toRunID
		})
		if keepIndex < 0 {
			return nil, -1, protocol.ErrRunNotFound
		}
	}
	return roots, keepIndex, nil
}

func describeDroppedRuns(
	material Material,
	droppedRoots []rundomain.Record,
) ([]protocol.DroppedRun, []string, []string, error) {
	itemsByRun := make(map[string][]protocol.Item)
	for _, record := range material.Items {
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, nil, nil, err
		}
		itemsByRun[record.RunID] = append(itemsByRun[record.RunID], item)
	}

	dropped := make([]protocol.DroppedRun, 0)
	dropRootIDs := make([]string, 0, len(droppedRoots))
	dropRunIDs := make([]string, 0)
	for _, root := range droppedRoots {
		dropRootIDs = append(dropRootIDs, root.Run.ID())
		for _, record := range material.Runs {
			if record.Run.ID() != root.Run.ID() && record.Run.RootRunID() != root.Run.ID() {
				continue
			}
			view, err := presentMaterialRun(record)
			if err != nil {
				return nil, nil, nil, err
			}
			entry := protocol.DroppedRun{Run: view.RunSummary}
			for _, item := range itemsByRun[record.Run.ID()] {
				if item.Type == protocol.ItemTypeUserMessage {
					entry.UserInput = slices.Clone(item.Content)
					break
				}
			}
			dropped = append(dropped, entry)
			dropRunIDs = append(dropRunIDs, record.Run.ID())
		}
	}
	return dropped, dropRootIDs, dropRunIDs, nil
}

func rollbackPlan(
	material Material,
	roots []rundomain.Record,
	keepIndex int,
	now time.Time,
) (*plandomain.State, error) {
	steps := []plandomain.Step(nil)
	if keepIndex >= 0 {
		if boundary, recorded := material.PlanBoundaries[roots[keepIndex].Run.ID()]; recorded {
			steps = boundary.Steps()
		}
	}
	if material.Plan.Revision() == 0 && len(steps) == 0 {
		return nil, nil
	}
	replacement, err := material.Plan.Replace(steps, now)
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}
