package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// turnLifecycle captures the first terminal process event the agent
// runtime publishes for a turn's ROOT process. The lifecycle listener
// wires into [agentexec.TurnRequest.EventListener] so the runtime fans
// every event for the turn through capture; runTurn reads the captured
// event after process.Done() to decide the TurnEnd reason.
//
// Sub-agent (subtask) processes now share this listener via runtime
// EventListener inheritance — their events arrive here too, tagged with
// their own ProcessID. A subtask runs synchronously inside the root's
// tool loop and therefore completes BEFORE the root, so its terminal
// event would pre-empt the root's under earliest-wins. The listener binds the
// root from the first ProcessCreated event, which the engine publishes
// synchronously before it starts the root goroutine, so the capture gate is in
// place before a child can emit anything.
//
// Only terminal events are kept — non-terminal events (PlanningStarted,
// ActionStarted, etc.) are dropped to keep the listener
// allocation-free in the hot path. Earliest-wins among the root's own
// terminals: a race between e.g. ProcessKilled (from Kill) and
// the run loop's exit ProcessFailed yields whichever arrived first.
type turnLifecycle struct {
	mu        sync.Mutex
	rootID    string // turn's root process id; empty until the first ProcessCreated
	terminal  event.Event
	sessionID string
	cwd       string
	hooks     *hooks.Bound
	subagents map[string]hooks.SubagentInput
}

// setRoot is a fallback for engine stubs and restored processes that do not
// publish ProcessCreated through the listener. The production start path has
// already bound the same id synchronously by the time StartTurn returns.
func (l *turnLifecycle) setRoot(id string) {
	l.mu.Lock()
	if l.rootID == "" {
		l.rootID = id
	}
	l.mu.Unlock()
}

func (l *turnLifecycle) listener(turnID string) *event.NamedListener {
	return event.NewNamedListener("turn-lifecycle-"+turnID, func(ctx context.Context, e event.Event) {
		if _, created := e.(event.ProcessCreated); created && l.bindRoot(e.ProcessID()) {
			return
		}
		l.fireSubagentHook(ctx, e)
		switch e.(type) {
		case event.ProcessCompleted,
			event.ProcessKilled,
			event.ProcessFailed,
			event.ProcessTerminated,
			event.ProcessStuck:
			l.mu.Lock()
			if l.terminal == nil && l.rootID != "" && e.ProcessID() == l.rootID {
				l.terminal = e
			}
			l.mu.Unlock()
		}
	})
}

func (l *turnLifecycle) bindRoot(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rootID != "" {
		return false
	}
	l.rootID = id
	return true
}

func (l *turnLifecycle) fireSubagentHook(ctx context.Context, e event.Event) {
	if l.hooks.Empty() {
		return
	}
	l.mu.Lock()
	rootID := l.rootID
	l.mu.Unlock()
	if rootID == "" || e.ProcessID() == rootID {
		return
	}
	switch ev := e.(type) {
	case event.ProcessCreated:
		in := hooks.SubagentInput{ProcessID: e.ProcessID(), ParentProcessID: rootID}
		in.Description, in.Prompt = subagentTaskInput(ev.Bindings)
		l.mu.Lock()
		if l.subagents == nil {
			l.subagents = map[string]hooks.SubagentInput{}
		}
		l.subagents[e.ProcessID()] = in
		l.mu.Unlock()
		_ = l.hooks.Run(ctx, hooks.Input{
			Event:     hooks.SubagentStart,
			SessionID: l.sessionID,
			Cwd:       l.cwd,
			Subagent:  &in,
		})
	case event.ProcessCompleted:
		l.runSubagentStopHook(ctx, e, "completed", summarizeHookValue(ev.Result), "")
	case event.ProcessFailed:
		l.runSubagentStopHook(ctx, e, "failed", "", errorString(ev.Err))
	case event.ProcessKilled:
		l.runSubagentStopHook(ctx, e, "killed", "", ev.Reason)
	case event.ProcessTerminated:
		l.runSubagentStopHook(ctx, e, "terminated", "", ev.Reason)
	case event.ProcessStuck:
		l.runSubagentStopHook(ctx, e, "stuck", "", "")
	}
}

func (l *turnLifecycle) runSubagentStopHook(ctx context.Context, e event.Event, status, result, errText string) {
	in := hooks.SubagentInput{ProcessID: e.ProcessID()}
	l.mu.Lock()
	if l.subagents != nil {
		if cached, ok := l.subagents[e.ProcessID()]; ok {
			in = cached
			delete(l.subagents, e.ProcessID())
		}
	}
	l.mu.Unlock()
	in.Status = status
	in.Result = result
	in.Error = errText
	_ = l.hooks.Run(ctx, hooks.Input{
		Event:     hooks.SubagentStop,
		SessionID: l.sessionID,
		Cwd:       l.cwd,
		Subagent:  &in,
		Reason:    string(e.Kind()),
	})
}

type subagentTask interface {
	SubagentDescription() string
	SubagentPrompt() string
}

func subagentTaskInput(bindings core.Bindings) (description, prompt string) {
	value, ok := bindings.Get(core.DefaultBindingName)
	if !ok {
		return "", ""
	}
	task, ok := value.(subagentTask)
	if !ok {
		return "", ""
	}
	return task.SubagentDescription(), summarizeHookText(task.SubagentPrompt())
}

func summarizeHookValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return summarizeHookText(x)
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return summarizeHookText(fmt.Sprint(x))
		}
		return summarizeHookText(string(b))
	}
}

func summarizeHookText(s string) string {
	const maxHookText = 2000
	if len(s) <= maxHookText {
		return s
	}
	end := 0
	for i := range s {
		if i > maxHookText {
			break
		}
		end = i
	}
	return s[:end] + "...(truncated)"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (l *turnLifecycle) terminalEvent() event.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.terminal
}
