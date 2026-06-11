package chat

import (
	"sync"

	"github.com/Tangerg/lynx/agent/event"
)

// turnLifecycle captures the first terminal process event the agent
// runtime publishes for a turn's ROOT process. The lifecycle listener
// wires into [engine.RunChatRequest.EventListener] so the runtime fans
// every event for the turn through capture; runTurn reads the captured
// event after proc.Done() to decide the TurnEnd reason.
//
// Sub-agent (subtask) processes now share this listener via runtime
// EventListener inheritance — their events arrive here too, tagged with
// their own ProcessID. A subtask runs synchronously inside the root's
// tool loop and therefore completes BEFORE the root, so its terminal
// event would pre-empt the root's under earliest-wins. setRoot records
// the root process id so the capture gate ignores any terminal whose
// ProcessID isn't the root's.
//
// Only terminal events are kept — non-terminal events (ReadyToPlan,
// ActionExecutionStart, etc.) are dropped to keep the listener
// allocation-free in the hot path. Earliest-wins among the root's own
// terminals: a race between e.g. ProcessKilled (from KillProcess) and
// the run loop's exit ProcessFailed yields whichever arrived first.
type turnLifecycle struct {
	mu       sync.Mutex
	rootID   string // turn's root process id; empty until setRoot
	terminal event.Event
}

// setRoot records the turn's root process id once StartChat returns it,
// so the listener can tell the root's terminal apart from a subtask's.
// Called before the root reaches any terminal state, so no terminal is
// missed by the gate.
func (l *turnLifecycle) setRoot(id string) {
	l.mu.Lock()
	l.rootID = id
	l.mu.Unlock()
}

func (l *turnLifecycle) listener(turnID string) *event.NamedListener {
	return event.NewNamedListener("chat-lifecycle-"+turnID, func(e event.Event) {
		switch e.(type) {
		case event.ProcessCompleted,
			event.ProcessKilled,
			event.ProcessFailed,
			event.ProcessTerminated,
			event.ProcessStuck:
			l.mu.Lock()
			// Only the root process's terminal decides TurnEnd. rootID == ""
			// (StartChat hasn't returned, or stub tests that never set it)
			// falls back to accepting any, preserving prior behavior.
			if l.terminal == nil && (l.rootID == "" || e.ProcessID() == l.rootID) {
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
