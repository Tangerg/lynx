package runtimeembedded

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type snapshotBinding interface {
	GetSession(context.Context, protocol.GetSessionRequest, embedded.CallOptions) (*protocol.Session, error)
	GetPlan(context.Context, protocol.GetPlanRequest, embedded.CallOptions) (*protocol.StateSnapshot, error)
	ListItems(context.Context, protocol.ListItemsRequest, embedded.CallOptions) (*protocol.ListItemsResponse, error)
	ListInterrupts(context.Context, protocol.ListInterruptsRequest, embedded.CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error)
}

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
			projected, err := projectSnapshot(current)
			if err != nil {
				return agent.SessionSnapshot{}, runtimeContractViolation("get session projection is invalid: %v", err)
			}
			return projected, nil
		}
		previous = current
	}
	return agent.SessionSnapshot{}, fmt.Errorf("%w: session %s changed throughout cold recovery", agent.ErrDisconnected, sessionID)
}

func (r *Runtime) readCold(ctx context.Context, sessionID string) (coldRead, error) {
	session, err := r.snapshot.GetSession(ctx, protocol.GetSessionRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return coldRead{}, classifyError(err)
	}
	if session == nil {
		return coldRead{}, runtimeContractViolation("get session returned nil")
	}
	runs, err := r.listAllRuns(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	items, err := r.listAllItems(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	plan, err := r.readPlan(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	interrupts, err := r.listAllInterrupts(ctx, sessionID)
	if err != nil {
		return coldRead{}, err
	}
	return coldRead{session: *session, runs: runs, items: items, plan: plan, interrupts: interrupts}, nil
}

func (r *Runtime) readPlan(ctx context.Context, sessionID string) (*protocol.StateSnapshot, error) {
	if !r.profile.Supports(runtimeprofile.FeaturePlan) {
		return nil, nil
	}
	plan, err := r.snapshot.GetPlan(ctx, protocol.GetPlanRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if plan == nil {
		return nil, runtimeContractViolation("get plan returned nil while the plan feature is enabled")
	}
	return plan, nil
}

func (r *Runtime) listAllRuns(ctx context.Context, sessionID string) ([]protocol.RunRef, error) {
	var values []protocol.RunRef
	cursors := newCursorTraversal("list runs", "")
	for {
		cursor := cursors.Current()
		page, err := r.runCatalog.ListRuns(ctx, protocol.ListRunsRequest{
			SessionID: sessionID, IncludeDescendants: true,
			PageQuery: protocol.PageQuery{Limit: 100, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, runtimeContractViolation("list runs returned a nil page")
		}
		values = append(values, page.Data...)
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			return values, nil
		}
	}
}

func (r *Runtime) listAllItems(ctx context.Context, sessionID string) ([]protocol.Item, error) {
	var values []protocol.Item
	cursors := newCursorTraversal("list items", "")
	for {
		cursor := cursors.Current()
		page, err := r.snapshot.ListItems(ctx, protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: sessionID},
			Order: protocol.ItemOrderAsc, PageQuery: protocol.PageQuery{Limit: 200, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, runtimeContractViolation("list items returned a nil page")
		}
		values = append(values, page.Data...)
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			return values, nil
		}
	}
}

func (r *Runtime) listAllInterrupts(ctx context.Context, sessionID string) ([]protocol.PendingInterruptSet, error) {
	var values []protocol.PendingInterruptSet
	cursors := newCursorTraversal("list interrupts", "")
	for {
		cursor := cursors.Current()
		page, err := r.snapshot.ListInterrupts(ctx, protocol.ListInterruptsRequest{
			SessionID: sessionID, PageQuery: protocol.PageQuery{Limit: 100, Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, runtimeContractViolation("list interrupts returned a nil page")
		}
		values = append(values, page.Data...)
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			return values, nil
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
	if read.plan != nil {
		snapshot.Plan, snapshot.PlanRevision, err = projectPlan(read.plan)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
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
