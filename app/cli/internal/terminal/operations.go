package terminal

import (
	"context"
	"sync"
)

type operationSlot string

const (
	streamOperation           operationSlot = "stream"
	completionOperation       operationSlot = "completion"
	searchOperation           operationSlot = "search"
	readerSearchOperation     operationSlot = "reader-search"
	pickerCatalogOperation    operationSlot = "picker-catalog"
	sessionCenterOperation    operationSlot = "session-center"
	sessionChangeOperation    operationSlot = "session-change"
	sessionOutputOperation    operationSlot = "session-output"
	approvalCatalogOperation  operationSlot = "approval-catalog"
	cancelRunOperation        operationSlot = "cancel-run"
	workspaceQueryOperation   operationSlot = "workspace-query"
	workspaceChangesOperation operationSlot = "workspace-changes"
)

type operationLease struct {
	slot operationSlot
	id   uint64
}

type ownedOperation struct {
	id     uint64
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
	o.active[slot] = ownedOperation{id: lease.id, cancel: cancel}
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

func runOperation[T any](a *app, slot operationSlot, replace bool, work func(context.Context) (T, error), apply func(T, error)) bool {
	dispatcher := a.loop.Dispatcher()
	return a.operations.Go(slot, replace, func(ctx context.Context, lease operationLease) {
		result, err := work(ctx)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed {
				return
			}
			apply(result, err)
		})
	})
}
