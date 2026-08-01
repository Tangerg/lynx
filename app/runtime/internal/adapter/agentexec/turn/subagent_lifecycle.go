package turn

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
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
	childRun  func(string) (runs.ChildRunBinding, bool)
	project   func(string) (agentexec.SubagentProjection, bool)
}

func newSubagentLifecycle(
	sessionID string,
	cwd string,
	bound *hooks.Bound,
	childRun func(string) (runs.ChildRunBinding, bool),
	project func(string) (agentexec.SubagentProjection, bool),
) *subagentLifecycle {
	if !bound.Handles(hooks.SubagentStart, hooks.SubagentStop) {
		return nil
	}
	return &subagentLifecycle{sessionID: sessionID, cwd: cwd, hooks: bound, childRun: childRun, project: project}
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
	switch l.rootID {
	case "":
		l.rootID = id
		return nil
	case id:
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
	if l.childRun == nil {
		return
	}
	binding, ok := l.childRun(e.ProcessID())
	if !ok {
		return
	}
	switch ev := e.(type) {
	case event.ProcessCreated:
		in := l.subagentInput(binding, "")
		_ = l.hooks.Run(ctx, hooks.Input{
			Event:     hooks.SubagentStart,
			SessionID: l.sessionID,
			Cwd:       l.cwd,
			Subagent:  &in,
		})
	case event.ProcessCompleted:
		l.runSubagentStopHook(ctx, binding, hooks.SubagentCompleted, "")
	case event.ProcessFailed:
		l.runSubagentStopHook(ctx, binding, hooks.SubagentFailed, errorString(ev.Err))
	case event.ProcessKilled:
		l.runSubagentStopHook(ctx, binding, hooks.SubagentKilled, ev.Reason)
	case event.ProcessTerminated:
		l.runSubagentStopHook(ctx, binding, hooks.SubagentTerminated, ev.Reason)
	case event.ProcessStuck:
		l.runSubagentStopHook(ctx, binding, hooks.SubagentStuck, "")
	}
}

// projection reads the host's view of a delegated process. A lifecycle built
// without one still fires its hooks, just without the task detail.
func (l *subagentLifecycle) projection(processID string) (agentexec.SubagentProjection, bool) {
	if l == nil || l.project == nil {
		return agentexec.SubagentProjection{}, false
	}
	return l.project(processID)
}

// subagentInput reads the delegated process into a hook payload. status decides
// whether a reply belongs in it: only a subagent that reached its goal has one,
// so every other terminal status describes a process that stopped before
// producing an answer. The start hook passes the zero status.
func (l *subagentLifecycle) subagentInput(binding runs.ChildRunBinding, status hooks.SubagentStatus) hooks.SubagentInput {
	in := hooks.SubagentInput{RunID: binding.RunID, ParentRunID: binding.ParentRunID, Status: status}
	if projection, ok := l.projection(binding.ProcessID); ok {
		in.Description = projection.Description
		in.Prompt = summarizeHookText(projection.Prompt)
		if status == hooks.SubagentCompleted {
			in.Result = summarizeHookText(projection.Reply)
		}
	}
	return in
}

func (l *subagentLifecycle) runSubagentStopHook(ctx context.Context, binding runs.ChildRunBinding, status hooks.SubagentStatus, errText string) {
	in := l.subagentInput(binding, status)
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
