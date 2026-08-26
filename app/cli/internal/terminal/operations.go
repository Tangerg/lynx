package terminal

import (
	"context"
	"sync"
)

type operationSlot string

const (
	streamOperation               operationSlot = "stream"
	pendingRunRecoveryOperation   operationSlot = "pending-run-recovery"
	pendingRunSettlementOperation operationSlot = "pending-run-settlement"
	resumeSettlementOperation     operationSlot = "resume-settlement"
	resumeRecoveryOperation       operationSlot = "resume-recovery"
	ownershipSettlementOperation  operationSlot = "ownership-settlement"
	completionOperation           operationSlot = "completion"
	searchOperation               operationSlot = "search"
	readerSearchOperation         operationSlot = "reader-search"
	readerDocumentOperation       operationSlot = "reader-document"
	pickerCatalogOperation        operationSlot = "picker-catalog"
	approvalModeOperation         operationSlot = "approval-mode"
	sessionCenterOperation        operationSlot = "session-center"
	sessionChangeOperation        operationSlot = "session-change"
	sessionOutputOperation        operationSlot = "session-output"
	approvalRuleOperation         operationSlot = "approval-rule"
	cancelRunOperation            operationSlot = "cancel-run"
	steerRunOperation             operationSlot = "steer-run"
	workspaceQueryOperation       operationSlot = "workspace-query"
	runtimeChangesOperation       operationSlot = "runtime-changes"
	sessionInvalidationOperation  operationSlot = "session-invalidation"
	modelConfigOperation          operationSlot = "model-config"
	goalOperation                 operationSlot = "goal"
	skillOperation                operationSlot = "skill"
	mcpOperation                  operationSlot = "mcp"
	mcpAuthorizationOperation     operationSlot = "mcp-authorization"
	scheduleOperation             operationSlot = "schedule"
	agentMemoryOperation          operationSlot = "agent-memory"
	knowledgeOperation            operationSlot = "knowledge"
	diagnosticToolOperation       operationSlot = "diagnostic-tool"
	authoringContextOperation     operationSlot = "authoring-context"
	hookOperation                 operationSlot = "hook"
	feedbackOperation             operationSlot = "feedback"
)

type operationLease struct {
	slot operationSlot
	id   uint64
}

type operationScope uint8

const (
	applicationOperationScope operationScope = iota
	sessionOperationScope
)

type runAdmissionPolicy uint8

const runAdmissionAfterSettlement runAdmissionPolicy = 1

type operationPolicy struct {
	scope        operationScope
	runAdmission runAdmissionPolicy
}

type ownedOperation struct {
	id     uint64
	policy operationPolicy
	cancel context.CancelFunc
}

// operationOwner gives every asynchronous adapter task a lifetime and, where
// appropriate, a replaceable slot. Closing it first rejects new work, then
// cancels and joins every cooperative worker.
type operationOwner struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	next   uint64
	active map[operationSlot]ownedOperation
	closed bool
	wg     sync.WaitGroup
}

func newOperationOwner(parent context.Context) *operationOwner {
	ctx, cancel := context.WithCancel(parent)
	return &operationOwner{ctx: ctx, cancel: cancel, active: make(map[operationSlot]ownedOperation)}
}

// Go starts work in slot. A replaceable operation cancels the previous owner;
// an exclusive operation is rejected while the slot is occupied.
func (o *operationOwner) Go(slot operationSlot, replace bool, work func(context.Context, operationLease)) bool {
	return o.goWithPolicy(operationPolicy{scope: applicationOperationScope}, slot, replace, work)
}

// GoSession starts work owned by the current terminal session projection. A
// successful projection replacement cancels all work in this scope before the
// new session becomes visible, so a late result cannot bleed into it.
func (o *operationOwner) GoSession(slot operationSlot, replace bool, work func(context.Context, operationLease)) bool {
	return o.goWithPolicy(operationPolicy{scope: sessionOperationScope}, slot, replace, work)
}

// GoSessionSettlement owns recovery of an accepted command whose local durable
// ownership has not settled yet. A later Run must not cross that boundary: the
// old command still occupies the session's single authoring outbox until this
// lease is released or the whole session projection is replaced.
func (o *operationOwner) GoSessionSettlement(
	slot operationSlot,
	replace bool,
	work func(context.Context, operationLease),
) bool {
	return o.goWithPolicy(operationPolicy{
		scope: sessionOperationScope, runAdmission: runAdmissionAfterSettlement,
	}, slot, replace, work)
}

func (o *operationOwner) goWithPolicy(
	policy operationPolicy,
	slot operationSlot,
	replace bool,
	work func(context.Context, operationLease),
) bool {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return false
	}
	if previous, exists := o.active[slot]; exists {
		if !replace {
			o.mu.Unlock()
			return false
		}
		previous.cancel()
	}
	ctx, cancel := context.WithCancel(o.ctx)
	o.next++
	lease := operationLease{slot: slot, id: o.next}
	o.active[slot] = ownedOperation{id: lease.id, policy: policy, cancel: cancel}
	o.wg.Go(func() {
		defer o.release(lease, cancel)
		work(ctx, lease)
	})
	o.mu.Unlock()
	return true
}

func (o *operationOwner) Current(lease operationLease) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	active, ok := o.active[lease.slot]
	return !o.closed && ok && active.id == lease.id
}

func (o *operationOwner) Active(slot operationSlot) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.active[slot]
	return !o.closed && ok
}

// BlocksRunAdmission reports whether an admitted runtime mutation must settle
// before the next queued prompt may become a Run. The policy belongs to the
// operation rather than to call sites that happen to initiate runs, so every
// dispatch path observes the same ordering boundary.
func (o *operationOwner) BlocksRunAdmission() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	for _, operation := range o.active {
		if operation.policy.runAdmission == runAdmissionAfterSettlement {
			return true
		}
	}
	return false
}

func (o *operationOwner) Cancel(slot operationSlot) {
	o.mu.Lock()
	active, ok := o.active[slot]
	if ok {
		delete(o.active, slot)
	}
	o.mu.Unlock()
	if ok {
		active.cancel()
	}
}

// CancelScope atomically retires every operation in scope before notifying its
// workers. Removing all leases under one lock is what prevents a completed old
// worker from winning a race with a newly installed session projection.
func (o *operationOwner) CancelScope(scope operationScope) {
	o.mu.Lock()
	operations := make([]ownedOperation, 0, len(o.active))
	for slot, operation := range o.active {
		if operation.policy.scope != scope {
			continue
		}
		delete(o.active, slot)
		operations = append(operations, operation)
	}
	o.mu.Unlock()
	for _, operation := range operations {
		operation.cancel()
	}
}

// Release relinquishes an exact operation lease. It is used when applying a
// completed result may synchronously start the next coalesced operation in the
// same slot; a stale worker can never release its successor.
func (o *operationOwner) Release(lease operationLease) bool {
	o.mu.Lock()
	active, ok := o.active[lease.slot]
	if ok && active.id == lease.id {
		delete(o.active, lease.slot)
	}
	o.mu.Unlock()
	if ok && active.id == lease.id {
		active.cancel()
		return true
	}
	return false
}

func (o *operationOwner) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	active := make([]ownedOperation, 0, len(o.active))
	for _, operation := range o.active {
		active = append(active, operation)
	}
	clear(o.active)
	o.mu.Unlock()

	o.cancel()
	for _, operation := range active {
		operation.cancel()
	}
	o.wg.Wait()
}

func (o *operationOwner) release(lease operationLease, cancel context.CancelFunc) {
	cancel()
	o.mu.Lock()
	if active, ok := o.active[lease.slot]; ok && active.id == lease.id {
		delete(o.active, lease.slot)
	}
	o.mu.Unlock()
}

// runOperation owns a user-initiated task for the lifetime of the current
// session projection. Most commands belong here because their result is
// interpreted against the session and workspace that launched them.
func (a *app) runOperation[T any](slot operationSlot, replace bool, work func(context.Context) (T, error), apply func(T, error)) bool {
	return a.runOwnedOperation(operationPolicy{scope: sessionOperationScope}, slot, replace, work, apply)
}

// runSessionSettlement owns a session command whose durable acknowledgement
// boundary orders subsequent Run admission. Prompts may be authored and queued
// while it settles, but dispatch cannot overtake the command journal.
func (a *app) runSessionSettlement[T any](
	slot operationSlot,
	replace bool,
	work func(context.Context) (T, error),
	apply func(T, error),
) bool {
	return a.runOwnedOperation(operationPolicy{
		scope: sessionOperationScope, runAdmission: runAdmissionAfterSettlement,
	}, slot, replace, work, apply)
}

// runApplicationOperation owns work whose domain lifetime is independent of a
// chat session. It survives projection replacement but is still canceled and
// joined when the terminal closes. Callers must keep apply safe when any
// session-scoped presentation they opened has since been dismissed.
func (a *app) runApplicationOperation[T any](slot operationSlot, replace bool, work func(context.Context) (T, error), apply func(T, error)) bool {
	return a.runOwnedOperation(operationPolicy{scope: applicationOperationScope}, slot, replace, work, apply)
}

// runAdmissionMutation owns an application-level mutation whose settled
// state is an input to subsequently admitted Runs. Prompts may still be queued
// while it is active, but queue dispatch waits until apply has observed the
// mutation result, so a Run cannot race stale approval, provider, tool, or
// authored-context state.
func (a *app) runAdmissionMutation[T any](
	slot operationSlot,
	replace bool,
	work func(context.Context) (T, error),
	apply func(T, error),
) bool {
	return a.runOwnedOperation(operationPolicy{
		scope: applicationOperationScope, runAdmission: runAdmissionAfterSettlement,
	}, slot, replace, work, apply)
}

func (a *app) runOwnedOperation[T any](
	policy operationPolicy,
	slot operationSlot,
	replace bool,
	work func(context.Context) (T, error),
	apply func(T, error),
) bool {
	dispatcher := a.loop.Dispatcher()
	return a.operations.goWithPolicy(policy, slot, replace, func(ctx context.Context, lease operationLease) {
		result, err := work(ctx)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed || !a.operations.Release(lease) {
				return
			}
			apply(result, err)
			if policy.runAdmission == runAdmissionAfterSettlement {
				a.drainQueue()
			}
		})
	})
}
