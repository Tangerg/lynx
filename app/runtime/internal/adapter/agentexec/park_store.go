package agentexec

import (
	"context"

	"github.com/Tangerg/lynx/agent/toolloop"
)

// AsParkStore adapts a driver-level [toolloop.ParkStore] into kernel's
// domain-local [ParkStore] abstraction, keeping kernel configuration
// agnostic to the loop driver's concrete type.
func AsParkStore(store toolloop.ParkStore) ParkStore {
	if store == nil {
		return nil
	}
	return parkStoreFromToolloop{inner: store}
}

type parkStoreFromToolloop struct {
	inner toolloop.ParkStore
}

func (p parkStoreFromToolloop) Consume(ctx context.Context, conversationID string) (*ParkState, error) {
	state, err := p.inner.Consume(ctx, conversationID)
	if err != nil || state == nil {
		return nil, err
	}
	return &ParkState{
		Assistant: state.Assistant,
		Done:      state.Done,
	}, nil
}

func (p parkStoreFromToolloop) Write(ctx context.Context, conversationID string, state *ParkState) error {
	if state == nil || state.Assistant == nil {
		return nil
	}
	return p.inner.Write(ctx, conversationID, &toolloop.ParkState{
		Assistant: state.Assistant,
		Done:      state.Done,
	})
}

// asToolloopParkStore adapts [ParkStore] to the driver-level contract
// expected by [agent/runtime.ToolLoopPolicy].
func asToolloopParkStore(store ParkStore) toolloop.ParkStore {
	if store == nil {
		return nil
	}
	return parkStoreToToolloop{inner: store}
}

type parkStoreToToolloop struct {
	inner ParkStore
}

func (p parkStoreToToolloop) Consume(ctx context.Context, conversationID string) (*toolloop.ParkState, error) {
	state, err := p.inner.Consume(ctx, conversationID)
	if err != nil || state == nil {
		return nil, err
	}
	return &toolloop.ParkState{
		Assistant: state.Assistant,
		Done:      state.Done,
	}, nil
}

func (p parkStoreToToolloop) Write(ctx context.Context, conversationID string, state *toolloop.ParkState) error {
	if state == nil || state.Assistant == nil {
		return nil
	}
	return p.inner.Write(ctx, conversationID, &ParkState{
		Assistant: state.Assistant,
		Done:      state.Done,
	})
}
