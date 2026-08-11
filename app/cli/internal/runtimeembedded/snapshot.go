package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const snapshotStabilityAttempts = 8

type coldRead struct {
	session    protocol.Session
	runs       []protocol.RunRef
	items      []protocol.Item
	plan       *protocol.StateSnapshot
	interrupts []protocol.PendingInterruptSet
}

// GetSession builds one stable cold projection from independently paginated
// runtime resources. Two equal lifecycle reads are required because the public
// protocol intentionally exposes resources rather than a binding-specific
// transaction handle.
func (r *Runtime) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	previous, err := r.readCold(ctx, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	for range snapshotStabilityAttempts {
		current, err := r.readCold(ctx, sessionID)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
		if coldReadsAgree(previous, current) {
			return projectSnapshot(current)
		}
		previous = current
	}
	return agent.SessionSnapshot{}, fmt.Errorf("%w: session %s changed throughout cold recovery", agent.ErrDisconnected, sessionID)
}

func (r *Runtime) readCold(ctx context.Context, sessionID string) (coldRead, error) {
	session, err := r.binding.GetSession(ctx, protocol.GetSessionRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return coldRead{}, classifyError(err)
	}
	if session == nil {
		return coldRead{}, errors.New("get session: runtime returned nil")
	}
	runs, err := r.listAllRuns(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	items, err := r.listAllItems(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	plan, err := r.binding.GetPlan(ctx, protocol.GetPlanRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return coldRead{}, classifyError(err)
	}
	interrupts, err := r.listAllInterrupts(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	return coldRead{session: *session, runs: runs, items: items, plan: plan, interrupts: interrupts}, nil
}

func (r *Runtime) listAllRuns(ctx context.Context, sessionID string) ([]protocol.RunRef, error) {
	var values []protocol.RunRef
	cursor := ""
	for {
		page, err := r.binding.ListRuns(ctx, protocol.ListRunsRequest{
			SessionID: sessionID, IncludeDescendants: false,
			PageQuery: protocol.PageQuery{Limit: 100, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, errors.New("list runs: runtime returned a nil page")
		}
		values = append(values, page.Data...)
		if page.NextCursor == "" {
			return values, nil
		}
		cursor, err = continueCursor("list runs", cursor, page.NextCursor)
		if err != nil {
			return nil, err
		}
	}
}

func (r *Runtime) listAllItems(ctx context.Context, sessionID string) ([]protocol.Item, error) {
	var values []protocol.Item
	cursor := ""
	for {
		page, err := r.binding.ListItems(ctx, protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: sessionID},
			Order: protocol.ItemOrderAsc, PageQuery: protocol.PageQuery{Limit: 200, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, errors.New("list items: runtime returned nil")
		}
		values = append(values, page.Data...)
		if page.NextCursor == "" {
			return values, nil
		}
		cursor, err = continueCursor("list items", cursor, page.NextCursor)
		if err != nil {
			return nil, err
		}
	}
}

func (r *Runtime) listAllInterrupts(ctx context.Context, sessionID string) ([]protocol.PendingInterruptSet, error) {
	var values []protocol.PendingInterruptSet
	cursor := ""
	for {
		page, err := r.binding.ListInterrupts(ctx, protocol.ListInterruptsRequest{
			SessionID: sessionID, PageQuery: protocol.PageQuery{Limit: 100, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, errors.New("list interrupts: runtime returned a nil page")
		}
		values = append(values, page.Data...)
		if page.NextCursor == "" {
			return values, nil
		}
		cursor, err = continueCursor("list interrupts", cursor, page.NextCursor)
		if err != nil {
			return nil, err
		}
	}
}

func coldReadsAgree(first, second coldRead) bool {
	if !reflect.DeepEqual(first.session, second.session) ||
		!reflect.DeepEqual(first.items, second.items) ||
		!reflect.DeepEqual(first.plan, second.plan) ||
		!reflect.DeepEqual(first.interrupts, second.interrupts) ||
		len(first.runs) != len(second.runs) {
		return false
	}
	for index := range first.runs {
		if !sameRunLifecycle(first.runs[index], second.runs[index]) {
			return false
		}
	}
	return true
}

func sameRunLifecycle(first, second protocol.RunRef) bool {
	return reflect.DeepEqual(first.RunSummary, second.RunSummary) &&
		first.ActiveSegmentID == second.ActiveSegmentID &&
		reflect.DeepEqual(first.Limits, second.Limits) &&
		reflect.DeepEqual(first.ProtocolProfile, second.ProtocolProfile)
}

func projectSnapshot(read coldRead) (agent.SessionSnapshot, error) {
	session, err := projectSession(read.session)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	snapshot := agent.SessionSnapshot{Session: session, Transcript: make([]agent.Block, 0, len(read.items))}
	for _, value := range read.items {
		block, err := projectItem(value)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
		snapshot.Transcript = append(snapshot.Transcript, block)
	}
	snapshot.Runs = make([]agent.Run, 0, len(read.runs))
	for _, value := range slices.Backward(read.runs) {
		run, err := projectRun(value)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
		snapshot.Runs = append(snapshot.Runs, run)
	}
	snapshot.Plan, snapshot.PlanRevision, err = projectPlan(read.plan)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	if active, ok := snapshot.ActiveRun(); ok && active.Status == agent.RunStatusWaiting {
		sets := make([]protocol.PendingInterruptSet, 0, 1)
		for _, set := range read.interrupts {
			if set.RootRunID == active.ID {
				sets = append(sets, set)
			}
		}
		if len(sets) != 1 {
			return agent.SessionSnapshot{}, fmt.Errorf("waiting run %s has %d pending interrupt sets", active.ID, len(sets))
		}
		snapshot.Interactions, err = projectInteractions(sets[0].Interrupts)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
	} else if len(read.interrupts) != 0 {
		return agent.SessionSnapshot{}, fmt.Errorf("session %s has interrupts without a waiting root run", session.ID)
	}
	if err := snapshot.Validate(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("cold session projection: %w", err)
	}
	return snapshot, nil
}
