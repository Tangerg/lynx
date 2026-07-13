package sessions

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestMutationCompletionDetachesFromCallerCancellation(t *testing.T) {
	mutations := new(observingMutations)
	coordinator := New(Dependencies{Mutations: mutations})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := coordinator.completeMutationDetached(ctx, "ses_1"); err != nil {
		t.Fatalf("completeMutationDetached: %v", err)
	}
	if mutations.canceled {
		t.Fatal("mutation cleanup inherited caller cancellation")
	}
	if !mutations.bounded {
		t.Fatal("mutation cleanup context has no deadline")
	}
}

type observingMutations struct {
	canceled bool
	bounded  bool
}

func (*observingMutations) Record(context.Context, execution.WorkspaceMutation) error { return nil }

func (m *observingMutations) Complete(ctx context.Context, _ string) error {
	m.canceled = ctx.Err() != nil
	_, m.bounded = ctx.Deadline()
	return nil
}

func (*observingMutations) ListPending(context.Context) ([]execution.WorkspaceMutation, error) {
	return nil, nil
}
