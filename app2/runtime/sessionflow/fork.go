package sessionflow

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (service *Service) Fork(
	ctx context.Context,
	request protocol.ForkSessionRequest,
) (*ForkResult, error) {
	material, err := service.store.ReadSessionMaterial(ctx, session.ID(request.SessionID))
	if err != nil {
		return nil, projectLookup(err)
	}
	keptRuns, boundaryRunID, err := selectForkRuns(material.Runs, request.FromRunID)
	if err != nil {
		return nil, err
	}

	newSessionID, err := service.ids.New("ses_")
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = material.Session.Title() + " (fork)"
	}
	child, err := session.New(session.Create{
		ID:        session.ID(newSessionID),
		Title:     title,
		Workspace: material.Session.Workspace(),
		Selection: material.Session.Selection(),
		Now:       now,
	})
	if err != nil {
		return nil, err
	}

	runIDs, err := service.forkRunIDs(keptRuns)
	if err != nil {
		return nil, err
	}
	itemIDs, err := service.forkItemIDs(material.Items, runIDs)
	if err != nil {
		return nil, err
	}
	runs, err := forkRuns(keptRuns, newSessionID, runIDs, itemIDs)
	if err != nil {
		return nil, err
	}
	items, err := forkItems(material.Items, newSessionID, runIDs, itemIDs)
	if err != nil {
		return nil, err
	}

	plan, err := forkPlan(material, boundaryRunID, newSessionID, now)
	if err != nil {
		return nil, err
	}
	write := ForkWrite{
		Session:        child,
		Runs:           runs,
		Items:          items,
		Messages:       forkMessages(material.Messages, newSessionID, runIDs),
		Plan:           plan,
		PlanBoundaries: forkPlanBoundaries(material.PlanBoundaries, runIDs),
		ToolResults:    forkToolResults(material.ToolResults, newSessionID, itemIDs),
	}
	if err := service.store.CreateSessionFork(ctx, write); err != nil {
		return nil, err
	}

	resolved, err := service.workspaces.Resolve(ctx, child.Workspace().Path())
	if err != nil {
		return nil, err
	}
	return &ForkResult{
		Session:     present(child, resolved, session.StatusIdle),
		PlanChanged: plan != nil,
	}, nil
}

func selectForkRuns(
	records []rundomain.Record,
	fromRunID string,
) ([]rundomain.Record, string, error) {
	roots := make([]rundomain.Record, 0)
	byRoot := make(map[string][]rundomain.Record)
	for _, record := range records {
		if record.Run.Status() != rundomain.Finished {
			continue
		}
		rootID := record.Run.RootRunID()
		if rootID == "" {
			rootID = record.Run.ID()
			roots = append(roots, record)
		}
		byRoot[rootID] = append(byRoot[rootID], record)
	}
	slices.SortFunc(roots, compareRunRecords)
	if fromRunID == "" && len(roots) > 0 {
		fromRunID = roots[len(roots)-1].Run.ID()
	}

	selected := make([]rundomain.Record, 0)
	found := fromRunID == ""
	for _, root := range roots {
		group := byRoot[root.Run.ID()]
		slices.SortFunc(group, compareRunRecords)
		selected = append(selected, group...)
		if root.Run.ID() == fromRunID {
			found = true
			break
		}
	}
	if !found {
		return nil, "", protocol.ErrRunNotFound
	}
	return selected, fromRunID, nil
}

func compareRunRecords(left, right rundomain.Record) int {
	if order := left.Run.CreatedAt().Compare(right.Run.CreatedAt()); order != 0 {
		return order
	}
	return strings.Compare(left.Run.ID(), right.Run.ID())
}

func (service *Service) forkRunIDs(records []rundomain.Record) (map[string]string, error) {
	result := make(map[string]string, len(records))
	for _, record := range records {
		id, err := service.ids.New("run_")
		if err != nil {
			return nil, err
		}
		result[record.Run.ID()] = id
	}
	return result, nil
}

func (service *Service) forkItemIDs(
	records []transcript.Record,
	runIDs map[string]string,
) (map[string]string, error) {
	result := make(map[string]string)
	for _, record := range records {
		if runIDs[record.RunID] == "" {
			continue
		}
		id, err := service.ids.New("itm_")
		if err != nil {
			return nil, err
		}
		result[record.ID] = id
	}
	return result, nil
}

func forkRuns(
	records []rundomain.Record,
	sessionID string,
	runIDs map[string]string,
	itemIDs map[string]string,
) ([]rundomain.Record, error) {
	result := make([]rundomain.Record, 0, len(records))
	for _, record := range records {
		value := record.Run
		restored, err := rundomain.Rehydrate(rundomain.Restore{
			ID:              runIDs[value.ID()],
			SessionID:       sessionID,
			ParentRunID:     runIDs[value.ParentRunID()],
			RootRunID:       runIDs[value.RootRunID()],
			SpawnedByItemID: itemIDs[value.SpawnedByItemID()],
			Status:          value.Status(),
			Provider:        value.Provider(),
			Model:           value.Model(),
			Outcome:         value.Outcome(),
			Detail:          value.Detail(),
			CreatedAt:       value.CreatedAt(),
			UpdatedAt:       value.UpdatedAt(),
			FinishedAt:      value.FinishedAt(),
		})
		if err != nil {
			return nil, err
		}
		result = append(result, rundomain.Record{
			Run:  restored,
			Body: slices.Clone(record.Body),
		})
	}
	return result, nil
}

func forkItems(
	records []transcript.Record,
	sessionID string,
	runIDs map[string]string,
	itemIDs map[string]string,
) ([]transcript.Record, error) {
	result := make([]transcript.Record, 0, len(itemIDs))
	for _, record := range records {
		newID := itemIDs[record.ID]
		if newID == "" {
			continue
		}
		var item protocol.Item
		if err := json.Unmarshal(record.Body, &item); err != nil {
			return nil, err
		}
		item.ID = newID
		item.RunID = runIDs[item.RunID]
		body, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		result = append(result, transcript.Record{
			ID:         newID,
			SessionID:  sessionID,
			RunID:      item.RunID,
			Ordinal:    record.Ordinal,
			Body:       body,
			SearchText: record.SearchText,
			CreatedAt:  record.CreatedAt,
		})
	}
	return result, nil
}

func forkMessages(
	records []conversationdomain.Record,
	sessionID string,
	runIDs map[string]string,
) []conversationdomain.Record {
	result := make([]conversationdomain.Record, 0, len(records))
	for _, record := range records {
		runID := runIDs[record.RunID]
		if runID == "" {
			continue
		}
		result = append(result, conversationdomain.Record{
			SessionID: sessionID,
			RunID:     runID,
			Ordinal:   len(result),
			Body:      slices.Clone(record.Body),
		})
	}
	return result
}

func forkPlan(
	material Material,
	boundaryRunID string,
	sessionID string,
	now time.Time,
) (*plandomain.State, error) {
	boundary, recorded := material.PlanBoundaries[boundaryRunID]
	if !recorded || len(boundary.Steps()) == 0 {
		return nil, nil
	}
	empty, err := plandomain.New(sessionID)
	if err != nil {
		return nil, err
	}
	value, err := empty.Replace(boundary.Steps(), now)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func forkPlanBoundaries(
	boundaries map[string]plandomain.Boundary,
	runIDs map[string]string,
) map[string]plandomain.Boundary {
	result := make(map[string]plandomain.Boundary)
	for sourceRunID, newRunID := range runIDs {
		if boundary, recorded := boundaries[sourceRunID]; recorded {
			result[newRunID] = boundary
		}
	}
	return result
}

func forkToolResults(
	records []toolresult.Record,
	sessionID string,
	itemIDs map[string]string,
) []toolresult.Record {
	result := make([]toolresult.Record, 0, len(records))
	for _, record := range records {
		itemID := itemIDs[record.ItemID]
		if itemID == "" {
			continue
		}
		record.SessionID = sessionID
		record.ItemID = itemID
		result = append(result, record)
	}
	return result
}
