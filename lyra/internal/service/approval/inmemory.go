package approval

import (
	"context"
	"sync/atomic"
)

// New returns the single-process in-memory [Service]. Initial stance
// comes from mode — pass [ModeYolo] for environments where every tool
// call auto-passes (CI, smoke tests); production callers typically wire
// [ModeBalanced] or [ModeSafe].
func New(mode Mode) Service {
	s := &inMemory{}
	s.mode.Store(int32(mode))
	return s
}

// inMemory holds the stance in an atomic.Int32 — lock-free get/set.
type inMemory struct {
	mode atomic.Int32
}

func (s *inMemory) GetMode(_ context.Context) (Mode, error) {
	return Mode(s.mode.Load()), nil
}

func (s *inMemory) SetMode(_ context.Context, mode Mode) error {
	s.mode.Store(int32(mode))
	return nil
}
