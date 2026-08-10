package session

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
