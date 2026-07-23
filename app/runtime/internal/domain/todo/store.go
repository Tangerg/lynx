// Package todo is the model-facing task list: the agent's own working
// checklist for a session. It is a small domain: an ordered list of items, each
// pending / in_progress / completed, with optional blocked reason and next
// action. The list survives across turns (and restarts). The model owns the list
// through the todo_write tool (a full-list replace); this package holds the
// types, progress-integrity rules, and canonical textual rendering shared by
// the tool and the system-prompt
// injection.
package todo

import (
	"errors"
	"fmt"
	"strings"
)

// Status is a todo item's lifecycle state.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Valid reports whether s is a recognized status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted:
		return true
	default:
		return false
	}
}

// Item is one entry in the agent's working checklist.
type Item struct {
	Content       string
	Status        Status
	BlockedReason string
	NextAction    string
}

// ErrInvalid wraps the human-readable reason a proposed list breaks a
// progress-integrity rule. The todo_write tool feeds the reason back to the
// model (recoverable) rather than aborting the run.
var ErrInvalid = errors.New("todo: invalid update")

// Validate enforces the progress-integrity rules on a proposed replacement
// (next) against the current list (prev) — the guardrails that stop a model
// from faking progress:
//
//   - every item must have content;
//   - every status must be recognized;
//   - at most ONE item may be in_progress (focus, not "doing everything");
//   - completed items must not carry blocked_reason or next_action;
//   - at most ONE item may NEWLY become completed per update — honest
//     incremental completion: finish and mark one task, then the next,
//     instead of flipping the whole list to done in a single call.
//
// The completed delta is counted in aggregate (completed(next) −
// completed(prev)), so it is robust to reordering and content edits that a
// positional item-by-item diff would mishandle. Returns an [ErrInvalid]-
// wrapped error naming the broken rule, or nil when next is acceptable.
func Validate(prev, next []Item) error {
	inProgress, completedNext := 0, 0
	for _, it := range next {
		if strings.TrimSpace(it.Content) == "" {
			return fmt.Errorf("%w: content is required", ErrInvalid)
		}
		if !it.Status.Valid() {
			return fmt.Errorf("%w: unknown status %q (use pending / in_progress / completed)", ErrInvalid, it.Status)
		}
		if it.Status == StatusCompleted && (strings.TrimSpace(it.BlockedReason) != "" || strings.TrimSpace(it.NextAction) != "") {
			return fmt.Errorf("%w: completed items must not carry blocked_reason or next_action", ErrInvalid)
		}
		switch it.Status {
		case StatusInProgress:
			inProgress++
		case StatusCompleted:
			completedNext++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("%w: %d items marked in_progress — keep exactly one in_progress at a time", ErrInvalid, inProgress)
	}
	if completedNext-completedCount(prev) > 1 {
		return fmt.Errorf("%w: %d items newly marked completed in one update — finish and mark them one at a time", ErrInvalid, completedNext-completedCount(prev))
	}
	return nil
}

func completedCount(items []Item) int {
	n := 0
	for _, it := range items {
		if it.Status == StatusCompleted {
			n++
		}
	}
	return n
}

// Render formats items as a compact checklist for the system-prompt injection
// and the tool's confirmation. An empty list renders as "".
func Render(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Status.mark())
		b.WriteByte(' ')
		b.WriteString(it.Content)
		b.WriteByte('\n')
		if it.BlockedReason != "" {
			b.WriteString("    blocked: ")
			b.WriteString(it.BlockedReason)
			b.WriteByte('\n')
		}
		if it.NextAction != "" {
			b.WriteString("    next: ")
			b.WriteString(it.NextAction)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (s Status) mark() string {
	switch s {
	case StatusCompleted:
		return "[x]"
	case StatusInProgress:
		return "[~]"
	default:
		return "[ ]"
	}
}
