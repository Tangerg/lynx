package terminal

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestOperationOwnerReplacesJoinsAndRejectsWorkAfterClose(t *testing.T) {
	owner := newOperationOwner(t.Context())
	firstCanceled := make(chan struct{})
	if !owner.Go("latest", true, func(ctx context.Context, _ operationLease) {
		<-ctx.Done()
		close(firstCanceled)
	}) {
		t.Fatal("first operation was rejected")
	}
	secondDone := make(chan struct{})
	if !owner.Go("latest", true, func(context.Context, operationLease) {
		close(secondDone)
	}) {
		t.Fatal("replacement operation was rejected")
	}
	<-firstCanceled
	<-secondDone

	blocked := make(chan struct{})
	if !owner.Go("exclusive", false, func(ctx context.Context, _ operationLease) {
		<-ctx.Done()
		close(blocked)
	}) {
		t.Fatal("exclusive operation was rejected")
	}
	var ran atomic.Bool
	if owner.Go("exclusive", false, func(context.Context, operationLease) { ran.Store(true) }) {
		t.Fatal("second exclusive operation was accepted")
	}
	owner.Close()
	<-blocked
	if ran.Load() {
		t.Fatal("rejected operation ran")
	}
	if owner.Go("after-close", true, func(context.Context, operationLease) {}) {
		t.Fatal("operation was accepted after close")
	}
}

func TestOperationOwnerReleaseMakesExclusiveSlotAvailableToSuccessor(t *testing.T) {
	owner := newOperationOwner(t.Context())
	t.Cleanup(owner.Close)
	successorDone := make(chan struct{})
	if !owner.Go("serial", false, func(_ context.Context, lease operationLease) {
		if !owner.Release(lease) {
			t.Error("active lease was not released")
			return
		}
		if !owner.Go("serial", false, func(context.Context, operationLease) {
			close(successorDone)
		}) {
			t.Error("successor was rejected after release")
		}
	}) {
		t.Fatal("first operation was rejected")
	}
	<-successorDone
}

func TestOperationOwnerCancelsOnlyTheRequestedScope(t *testing.T) {
	owner := newOperationOwner(t.Context())
	t.Cleanup(owner.Close)
	applicationCanceled := make(chan struct{})
	if !owner.Go("application", false, func(ctx context.Context, _ operationLease) {
		<-ctx.Done()
		close(applicationCanceled)
	}) {
		t.Fatal("application operation was rejected")
	}
	sessionCanceled := make(chan struct{})
	if !owner.GoSession("session", false, func(ctx context.Context, _ operationLease) {
		<-ctx.Done()
		close(sessionCanceled)
	}) {
		t.Fatal("session operation was rejected")
	}

	owner.CancelScope(sessionOperationScope)
	<-sessionCanceled
	select {
	case <-applicationCanceled:
		t.Fatal("canceling the session scope canceled application work")
	default:
	}
	if owner.Active("session") {
		t.Fatal("canceled session operation remains active")
	}
	if !owner.Active("application") {
		t.Fatal("application operation was retired with the session scope")
	}
}

func TestOperationOwnerDerivesRunAdmissionFromTheCurrentLease(t *testing.T) {
	owner := newOperationOwner(t.Context())
	t.Cleanup(owner.Close)

	concurrentDone := make(chan struct{})
	if !owner.Go("query", false, func(ctx context.Context, _ operationLease) {
		defer close(concurrentDone)
		<-ctx.Done()
	}) {
		t.Fatal("concurrent operation was rejected")
	}
	if owner.BlocksRunAdmission() {
		t.Fatal("ordinary application work blocked run admission")
	}
	owner.Cancel("query")
	<-concurrentDone

	blockingDone := make(chan struct{})
	if !owner.goWithPolicy(operationPolicy{
		scope: applicationOperationScope, runAdmission: runAdmissionAfterSettlement,
	}, "mutation", false, func(ctx context.Context, _ operationLease) {
		defer close(blockingDone)
		<-ctx.Done()
	}) {
		t.Fatal("run-prerequisite mutation was rejected")
	}
	if !owner.BlocksRunAdmission() {
		t.Fatal("run-prerequisite mutation did not block run admission")
	}
	owner.Cancel("mutation")
	<-blockingDone
	if owner.BlocksRunAdmission() {
		t.Fatal("retired mutation kept run admission blocked")
	}
}

func TestSessionSettlementOwnsRunAdmissionUntilItsExactLeaseEnds(t *testing.T) {
	owner := newOperationOwner(t.Context())
	t.Cleanup(owner.Close)

	done := make(chan struct{})
	if !owner.GoSessionSettlement("outbox", false, func(ctx context.Context, _ operationLease) {
		defer close(done)
		<-ctx.Done()
	}) {
		t.Fatal("session settlement was rejected")
	}
	if !owner.BlocksRunAdmission() {
		t.Fatal("unsettled session ownership did not block a later run")
	}

	owner.CancelScope(sessionOperationScope)
	<-done
	if owner.BlocksRunAdmission() {
		t.Fatal("replaced session retained the old settlement boundary")
	}
}
