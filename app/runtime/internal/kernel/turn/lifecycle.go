package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// turnLifecycle captures the first terminal process event the agent
// runtime publishes for a turn's ROOT process. The lifecycle listener
// wires into [kernel.TurnRequest.EventListener] so the runtime fans
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
	mu        sync.Mutex
	rootID    string // turn's root process id; empty until setRoot
	terminal  event.Event
	sessionID string
	cwd       string
	hooks     *hooks.Bound
	subagents map[string]hooks.SubagentInput
}

// setRoot records the turn's root process id once StartTurn returns it,
// so the listener can tell the root's terminal apart from a subtask's.
// Called before the root reaches any terminal state, so no terminal is
// missed by the gate.
func (l *turnLifecycle) setRoot(id string) {
	l.mu.Lock()
	l.rootID = id
	l.mu.Unlock()
}

func (l *turnLifecycle) listener(turnID string) *event.NamedListener {
	return event.NewNamedListener("turn-lifecycle-"+turnID, func(ctx context.Context, e event.Event) {
		l.fireSubagentHook(ctx, e)
		switch e.(type) {
		case event.ProcessCompleted,
			event.ProcessKilled,
			event.ProcessFailed,
			event.ProcessTerminated,
			event.ProcessStuck:
			l.mu.Lock()
			// Only the root process's terminal decides TurnEnd. rootID == ""
			// (StartTurn hasn't returned, or stub tests that never set it)
			// falls back to accepting any, preserving prior behavior.
			if l.terminal == nil && (l.rootID == "" || e.ProcessID() == l.rootID) {
				l.terminal = e
			}
			l.mu.Unlock()
		}
	})
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
		Reason:    e.EventName(),
	})
}

func subagentTaskInput(bindings map[string]any) (description, prompt string) {
	if len(bindings) == 0 {
		return "", ""
	}
	input, ok := bindings[core.DefaultBindingName]
	if !ok {
		for _, value := range bindings {
			input = value
			break
		}
	}
	return stringField(input, "Description"), summarizeHookText(stringField(input, "Prompt"))
}

func stringField(v any, name string) string {
	if v == nil {
		return ""
	}
	if m, ok := v.(map[string]any); ok {
		if s, ok := m[name].(string); ok {
			return s
		}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	field := rv.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
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
	return s[:maxHookText] + "...(truncated)"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (l *turnLifecycle) get() event.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.terminal
}
