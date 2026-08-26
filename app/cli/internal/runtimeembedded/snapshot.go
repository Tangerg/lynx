package runtimeembedded

import (
	"cmp"
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
	GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest, embedded.CallOptions) (*protocol.SessionSnapshot, error)
}

const snapshotStabilityAttempts = 8

type coldRead struct {
	session    protocol.Session
	runs       []protocol.RunRef
	items      []protocol.Item
	plan       *protocol.Plan
	interrupts []protocol.PendingInterruptSet
}

// GetSession binds independently owned Session metadata to one transactionally
// coherent material snapshot. Identical metadata projections around the read
// prove that its lifecycle cannot belong to a different Session generation.
func (r *Runtime) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	previous, err := r.readSession(ctx, sessionID)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	for range snapshotStabilityAttempts {
		material, err := r.readMaterialSnapshot(ctx, sessionID)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
		current, err := r.readSession(ctx, sessionID)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
		if reflect.DeepEqual(previous, current) {
			material.session = current
			projected, err := projectSnapshot(material)
			if err != nil {
				return agent.SessionSnapshot{}, runtimeContractViolation("get session projection is invalid: %v", err)
			}
			return projected, nil
		}
		previous = current
	}
	return agent.SessionSnapshot{}, fmt.Errorf("%w: session %s changed throughout cold recovery", agent.ErrDisconnected, sessionID)
}

func (r *Runtime) readSession(ctx context.Context, sessionID string) (protocol.Session, error) {
	session, err := r.snapshot.GetSession(ctx, protocol.GetSessionRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return protocol.Session{}, classifyError(err)
	}
	if session == nil {
		return protocol.Session{}, runtimeContractViolation("get session returned nil")
	}
	return *session, nil
}

func (r *Runtime) readMaterialSnapshot(ctx context.Context, sessionID string) (coldRead, error) {
	snapshot, err := r.snapshot.GetSessionSnapshot(ctx, protocol.GetSessionSnapshotRequest{
		SessionID: sessionID, IncludeDescendants: r.profile.Supports(runtimeprofile.FeatureSubagents),
	}, r.callOptions())
	if err != nil {
		return coldRead{}, classifyError(err)
	}
	if snapshot == nil {
		return coldRead{}, runtimeContractViolation("get session snapshot returned nil")
	}
	planEnabled := r.profile.Supports(runtimeprofile.FeaturePlan)
	if planEnabled && snapshot.Plan == nil {
		return coldRead{}, runtimeContractViolation("get session snapshot omitted plan while the plan feature is enabled")
	}
	if !planEnabled && snapshot.Plan != nil {
		return coldRead{}, runtimeContractViolation("get session snapshot returned plan while the plan feature is disabled")
	}
	return coldRead{
		runs: snapshot.Runs, items: snapshot.Items, plan: snapshot.Plan, interrupts: snapshot.Interrupts,
	}, nil
}

func projectSnapshot(read coldRead) (agent.SessionSnapshot, error) {
	session, err := projectSession(read.session)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	snapshot := agent.SessionSnapshot{Session: session, Transcript: make([]agent.Block, 0, len(read.items))}
	for _, value := range read.items {
		block, projectItemErr := projectItem(value)
		if projectItemErr != nil {
			return agent.SessionSnapshot{}, projectItemErr
		}
		snapshot.Transcript = append(snapshot.Transcript, block)
	}
	orderedRuns := slices.Clone(read.runs)
	slices.SortFunc(orderedRuns, func(first, second protocol.RunRef) int {
		return cmp.Or(first.CreatedAt.Compare(second.CreatedAt), cmp.Compare(first.ID, second.ID))
	})
	snapshot.Runs = make([]agent.Run, 0, len(orderedRuns))
	for _, value := range orderedRuns {
		run, projectRunErr := projectRun(value)
		if projectRunErr != nil {
			return agent.SessionSnapshot{}, projectRunErr
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
