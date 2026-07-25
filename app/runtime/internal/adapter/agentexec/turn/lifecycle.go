package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// turnLifecycle routes process events to subagent lifecycle hooks. The root
// process identity prevents child events from being projected as root hooks.
//
// Sub-agent (subtask) processes now share this listener via runtime
// SubtreeEventListener inheritance — their events arrive here too, tagged with
// their own ProcessID. A subtask runs synchronously inside the root's
// tool loop and therefore completes BEFORE the root, so its terminal
// event would pre-empt the root's under earliest-wins. The listener binds the
// root from the first ProcessCreated event, which the engine publishes
// synchronously before it starts the root goroutine, so the capture gate is in
// place before a child can emit anything.
type turnLifecycle struct {
	mu        sync.Mutex
	rootID    string // turn's root process id; empty until the first ProcessCreated
	sessionID string
	cwd       string
	hooks     *hooks.Bound
	subagents map[string]hooks.SubagentInput
}

// confirmRoot binds a restored process, or verifies that the synchronous
// ProcessCreated event and the process returned by StartTurn identify the same
// root.
func (l *turnLifecycle) confirmRoot(id string) error {
	if id == "" {
		return errors.New("turn lifecycle: root process id is empty")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.rootID == "":
		l.rootID = id
		return nil
	case l.rootID == id:
		return nil
	default:
		return fmt.Errorf("turn lifecycle: created root %q differs from returned process %q", l.rootID, id)
	}
}

func (l *turnLifecycle) listener(turnID string) *event.NamedSubtreeListener {
	return event.NewNamedSubtreeListener("turn-lifecycle-"+turnID, func(ctx context.Context, e event.Event) {
		if _, created := e.(event.ProcessCreated); created && l.bindRoot(e.ProcessID()) {
			return
		}
		l.fireSubagentHook(ctx, e)
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
		l.runSubagentStopHook(ctx, e, hooks.SubagentCompleted, summarizeHookValue(ev.Result), "")
	case event.ProcessFailed:
		l.runSubagentStopHook(ctx, e, hooks.SubagentFailed, "", errorString(ev.Err))
	case event.ProcessKilled:
		l.runSubagentStopHook(ctx, e, hooks.SubagentKilled, "", ev.Reason)
	case event.ProcessTerminated:
		l.runSubagentStopHook(ctx, e, hooks.SubagentTerminated, "", ev.Reason)
	case event.ProcessStuck:
		l.runSubagentStopHook(ctx, e, hooks.SubagentStuck, "", "")
	}
}

func (l *turnLifecycle) runSubagentStopHook(ctx context.Context, e event.Event, status hooks.SubagentStatus, result, errText string) {
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
		Reason:    subagentStopReason(status),
	})
}

func subagentStopReason(status hooks.SubagentStatus) string {
	switch status {
	case hooks.SubagentCompleted:
		return "subagent completed"
	case hooks.SubagentFailed:
		return "subagent failed"
	case hooks.SubagentKilled:
		return "subagent was killed"
	case hooks.SubagentTerminated:
		return "subagent was terminated"
	case hooks.SubagentStuck:
		return "subagent became stuck"
	default:
		return "subagent stopped"
	}
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
