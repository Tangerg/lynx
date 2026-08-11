package runtimeembedded

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type snapshotBindingStub struct {
	runs       map[string]*protocol.Page[protocol.RunRef]
	items      map[string]*protocol.ListItemsResponse
	interrupts map[string]*protocol.Page[protocol.PendingInterruptSet]
}

func (snapshotBindingStub) GetSession(context.Context, protocol.GetSessionRequest, embedded.CallOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected GetSession")
}

func (snapshotBindingStub) GetPlan(context.Context, protocol.GetPlanRequest, embedded.CallOptions) (*protocol.StateSnapshot, error) {
	return nil, errors.New("unexpected GetPlan")
}

func (stub snapshotBindingStub) ListRuns(_ context.Context, request protocol.ListRunsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return stub.runs[request.Cursor], nil
}

func (stub snapshotBindingStub) ListItems(_ context.Context, request protocol.ListItemsRequest, _ embedded.CallOptions) (*protocol.ListItemsResponse, error) {
	return stub.items[request.Cursor], nil
}

func (stub snapshotBindingStub) ListInterrupts(_ context.Context, request protocol.ListInterruptsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return stub.interrupts[request.Cursor], nil
}

func TestSnapshotResourcesRejectMultiStepCursorCycles(t *testing.T) {
	t.Parallel()
	itemPage := func(next string) *protocol.ListItemsResponse {
		return &protocol.ListItemsResponse{Page: *protocol.NewPageWithCursor([]protocol.Item{}, next)}
	}
	tests := []struct {
		name string
		stub snapshotBindingStub
		call func(context.Context, *Runtime) error
	}{
		{
			name: "runs",
			stub: snapshotBindingStub{runs: map[string]*protocol.Page[protocol.RunRef]{
				"":       protocol.NewPageWithCursor([]protocol.RunRef{}, "first"),
				"first":  protocol.NewPageWithCursor([]protocol.RunRef{}, "second"),
				"second": protocol.NewPageWithCursor([]protocol.RunRef{}, "first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllRuns(ctx, "session")
				return err
			},
		},
		{
			name: "items",
			stub: snapshotBindingStub{items: map[string]*protocol.ListItemsResponse{
				"": itemPage("first"), "first": itemPage("second"), "second": itemPage("first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllItems(ctx, "session")
				return err
			},
		},
		{
			name: "interrupts",
			stub: snapshotBindingStub{interrupts: map[string]*protocol.Page[protocol.PendingInterruptSet]{
				"":       protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "first"),
				"first":  protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "second"),
				"second": protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllInterrupts(ctx, "session")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{snapshot: test.stub, meta: requestMeta("test")}
			err := test.call(t.Context(), runtime)
			if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
				t.Fatalf("snapshot list error = %v, want cursor cycle failure", err)
			}
		})
	}
}
