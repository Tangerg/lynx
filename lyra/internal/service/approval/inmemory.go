package approval

import (
	"context"
	"sync"
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

// rememberKey identifies one standing decision: a tool name within a
// session (AUX_API §6 — the remember key is the tool NAME, not its args).
type rememberKey struct {
	session string
	tool    string
}

// inMemory holds the stance in an atomic.Int32 (lock-free get/set) plus the
// session-scoped remembered decisions guarded by mu. The remembered map is
// write-rare (only on "approve/deny + remember"), read once per gated call.
type inMemory struct {
	mode atomic.Int32

	mu         sync.Mutex
	remembered map[rememberKey]bool // standing decision: approved?
}

func (s *inMemory) GetMode(_ context.Context) (Mode, error) {
	return Mode(s.mode.Load()), nil
}

func (s *inMemory) SetMode(_ context.Context, mode Mode) error {
	s.mode.Store(int32(mode))
	return nil
}

func (s *inMemory) Remember(_ context.Context, sessionID, toolName string, approved bool) error {
	if sessionID == "" || toolName == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remembered == nil {
		s.remembered = map[rememberKey]bool{}
	}
	s.remembered[rememberKey{sessionID, toolName}] = approved
	return nil
}

func (s *inMemory) Remembered(_ context.Context, sessionID, toolName string) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	approved, ok := s.remembered[rememberKey{sessionID, toolName}]
	return approved, ok, nil
}
