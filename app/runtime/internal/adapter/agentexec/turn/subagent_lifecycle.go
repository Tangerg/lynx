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

// subagentLifecycle projects child process events into configured subagent
// hooks. The root identity excludes the turn's own process from that projection.
type subagentLifecycle struct {
	mu        sync.Mutex
	rootID    string // turn's root process id; empty until the first ProcessCreated
	sessionID string
	cwd       string
	hooks     *hooks.Bound
	result    func(string) (any, bool)
	subagents map[string]hooks.SubagentInput
}

func newSubagentLifecycle(
	sessionID string,
	cwd string,
	bound *hooks.Bound,
	result func(string) (any, bool),
) *subagentLifecycle {
	if !bound.Handles(hooks.SubagentStart, hooks.SubagentStop) {
		return nil
	}
	return &subagentLifecycle{sessionID: sessionID, cwd: cwd, hooks: bound, result: result}
}

// confirmRoot binds a restored process, or verifies that the synchronous
// ProcessCreated event and the process returned by StartTurn identify the same
// root.
func (l *subagentLifecycle) confirmRoot(id string) error {
	if id == "" {
		return errors.New("subagent lifecycle: root process id is empty")
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
		return fmt.Errorf("subagent lifecycle: created root %q differs from returned process %q", l.rootID, id)
	}
}

func (l *subagentLifecycle) listener(turnID string) *event.NamedSubtreeListener {
	return event.NewNamedSubtreeListener("subagent-lifecycle-"+turnID, func(ctx context.Context, e event.Event) {
		if created, ok := e.(event.ProcessCreated); ok && created.ParentID == "" {
			if l.bindRoot(e.ProcessID()) {
				return
			}
		}
		l.fireSubagentHook(ctx, e)
	})
}

func (l *subagentLifecycle) bindRoot(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rootID != "" {
		return false
	}
	l.rootID = id
	return true
}

func (l *subagentLifecycle) fireSubagentHook(ctx context.Context, e event.Event) {
	l.mu.Lock()
	rootID := l.rootID
	l.mu.Unlock()
	if rootID == "" || e.ProcessID() == rootID {
		return
	}
	switch ev := e.(type) {
	case event.ProcessCreated:
		in := hooks.SubagentInput{ProcessID: e.ProcessID(), ParentProcessID: ev.ParentID}
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
		var result any
		if l.result != nil {
			result, _ = l.result(e.ProcessID())
		}
		l.runSubagentStopHook(ctx, e, hooks.SubagentCompleted, summarizeHookValue(result), "")
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

func (l *subagentLifecycle) runSubagentStopHook(ctx context.Context, e event.Event, status hooks.SubagentStatus, result, errText string) {
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
