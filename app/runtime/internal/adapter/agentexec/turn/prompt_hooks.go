package turn

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// runPromptHooks fires SessionStart (once per session per process) + the
// UserPromptSubmit hook before a turn starts. It returns the (possibly
// context-prefixed) message, or an error wrapping [ErrPromptBlocked] when a hook
// blocked the prompt.
func (s *memoryDispatcher) runPromptHooks(ctx context.Context, request StartTurnRequest, st *turnState) (string, error) {
	var blocked bool
	var reason, inject string
	add := func(d hooks.Decision) {
		if d.Block && !blocked {
			blocked, reason = true, d.Reason
		}
		if d.InjectContext != "" {
			if inject != "" {
				inject += "\n"
			}
			inject += d.InjectContext
		}
	}
	if s.firstTurnForSession(request.SessionID) {
		add(st.hooks.Run(ctx, hooks.Input{Event: hooks.SessionStart, SessionID: request.SessionID, Cwd: request.Cwd}))
	}
	add(st.hooks.Run(ctx, hooks.Input{
		Event: hooks.UserPromptSubmit, SessionID: request.SessionID, Cwd: request.Cwd, Prompt: request.Message,
	}))
	if blocked {
		if reason == "" {
			reason = "blocked by a hook"
		}
		return "", fmt.Errorf("%w: %s", ErrPromptBlocked, reason)
	}
	if inject != "" {
		return "<hook-context>\n" + inject + "\n</hook-context>\n\n" + request.Message, nil
	}
	return request.Message, nil
}

// firstTurnForSession reports whether this is the first turn the process has
// opened for sessionID (and records it); the SessionStart fire-once gate.
func (s *memoryDispatcher) firstTurnForSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seenSessions[sessionID]; ok {
		return false
	}
	s.seenSessions[sessionID] = struct{}{}
	return true
}

// ForgetSession drops sessionID's SessionStart fire-once marker on session
// delete, so the gate set doesn't leak one entry per session over the process
// lifetime. See [Dispatcher.ForgetSession].
func (s *memoryDispatcher) ForgetSession(sessionID string) {
	s.mu.Lock()
	delete(s.seenSessions, sessionID)
	s.mu.Unlock()
}
