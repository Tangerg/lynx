package chat

import (
	"sync"

	"github.com/Tangerg/lynx/agent/event"
)

// turnLifecycle captures the first terminal process event the agent
// runtime publishes for a turn. The lifecycle listener wires into
// [engine.RunChatRequest.EventListener] so the platform multicast
// fans every event for the process through capture; runTurn reads
// the captured event after proc.Done() to decide the TurnEnd reason.
//
// Only terminal events are kept — non-terminal events (ReadyToPlan,
// ActionExecutionStart, etc.) are dropped to keep the listener
// allocation-free in the hot path. Earliest-wins: a race between
// e.g. ProcessKilled (from KillProcess) and the run loop's exit
// ProcessFailed yields whichever the multicast delivered first.
type turnLifecycle struct {
	mu       sync.Mutex
	terminal event.Event
}

func (l *turnLifecycle) listener(turnID string) *event.NamedListener {
	return event.NewNamedListener("lyra-chat-lifecycle-"+turnID, func(e event.Event) {
		switch e.(type) {
		case event.ProcessCompleted,
			event.ProcessKilled,
			event.ProcessFailed,
			event.ProcessTerminated,
			event.ProcessStuck:
			l.mu.Lock()
			if l.terminal == nil {
				l.terminal = e
			}
			l.mu.Unlock()
		}
	})
}

func (l *turnLifecycle) get() event.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.terminal
}
