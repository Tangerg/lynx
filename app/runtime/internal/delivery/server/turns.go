package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type runtimeTurns struct {
	rt turnAccess
}

func (s *Server) turns() runtimeTurns {
	return runtimeTurns{rt: s.rt}
}

func (t runtimeTurns) Cancel(ctx context.Context, handle turn.TurnHandle) error {
	return t.rt.CancelTurn(ctx, handle)
}

func (t runtimeTurns) Resume(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return t.rt.ResumeTurn(ctx, handle, resolution)
}

func (t runtimeTurns) Rehydrate(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return t.rt.RehydrateTurn(ctx, req)
}

func (t runtimeTurns) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return t.rt.TurnProcessID(ctx, handle)
}
