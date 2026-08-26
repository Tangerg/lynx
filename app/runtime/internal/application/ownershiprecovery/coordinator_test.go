package ownershiprecovery_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/ownershiprecovery"
)

type testLease struct{ release func() }

func (t testLease) Release() { t.release() }

type testOwnership struct {
	acquired bool
	released int
}

func (t *testOwnership) TryRecoverySweep() (ownershiprecovery.Lease, bool) {
	if !t.acquired {
		return nil, false
	}
	return testLease{release: func() { t.released++ }}, true
}

func (t *testOwnership) AcquireRecoverySweep(context.Context) (ownershiprecovery.Lease, error) {
	if !t.acquired {
		return nil, errors.New("contended")
	}
	return testLease{release: func() { t.released++ }}, nil
}

type runReconciler func(context.Context) (int, error)

func (r runReconciler) Reconcile(ctx context.Context) (int, error) {
	return r(ctx)
}

type goalReconciler func(context.Context) error

func (g goalReconciler) Reconcile(ctx context.Context) error { return g(ctx) }

func TestCoordinatorElectsOneWinnerAndOrdersRunBeforeGoalRecovery(t *testing.T) {
	var order []string
	ownership := &testOwnership{acquired: true}
	coordinator, err := ownershiprecovery.New(
		runReconciler(func(context.Context) (int, error) {
			order = append(order, "runs")
			return 1, nil
		}),
		goalReconciler(func(context.Context) error {
			order = append(order, "goals")
			return nil
		}),
		ownership,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ReconcileStartup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"runs", "goals"}) || ownership.released != 1 {
		t.Fatalf("order=%v releases=%d", order, ownership.released)
	}
}

func TestCoordinatorSkipsContendedSweepAndReleasesAfterFailure(t *testing.T) {
	ownership := &testOwnership{}
	calls := 0
	coordinator, err := ownershiprecovery.New(
		runReconciler(func(context.Context) (int, error) {
			calls++
			return 0, errors.New("failed")
		}),
		nil,
		ownership,
	)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Reconcile(t.Context())
	if err != nil || acquired || calls != 0 {
		t.Fatalf("contended sweep: acquired=%t calls=%d err=%v", acquired, calls, err)
	}
	ownership.acquired = true
	acquired, err = coordinator.Reconcile(t.Context())
	if !acquired || err == nil || ownership.released != 1 {
		t.Fatalf("failed sweep: acquired=%t releases=%d err=%v", acquired, ownership.released, err)
	}
}
