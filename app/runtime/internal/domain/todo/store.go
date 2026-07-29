// Package todo is the agent's working checklist domain: an ordered list of
// items, each pending / in_progress / completed, with optional blocked reason
// and next action. The list survives across turns and restarts; model-facing
// tool and prompt presentation live in adapters.
package todo

import (
	"errors"
	"fmt"
	"strings"
	"time"
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
	if err := ValidateSnapshot(next); err != nil {
		return err
	}
	completedNext := completedCount(next)
	if completedNext-completedCount(prev) > 1 {
		return fmt.Errorf("%w: %d items newly marked completed in one update — finish and mark them one at a time", ErrInvalid, completedNext-completedCount(prev))
	}
	return nil
}

// ValidateSnapshot verifies the invariant shape of a persisted todo list. It
// intentionally excludes transition rules such as the one-new-completion
// limit: a previously valid snapshot can contain many completed items.
func ValidateSnapshot(items []Item) error {
	inProgress, completedNext := 0, 0
	for _, it := range items {
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

// State is a session's whole task list as one durable latest-value projection:
// the items, the monotonic revision that produced them, and when it landed.
//
// Revision exists because the list is REPLACED wholesale. Two clients folding the
// same session cannot tell an older replacement from a newer one by content — the
// list can shrink, and a late-arriving older snapshot would look like progress
// undone. Zero is "never written", which is why it is not a timestamp: clocks tie
// and an imported session carries backdated ones.
type State struct {
	Items    []Item
	Revision uint64
	// UpdatedAt is absent (zero) exactly while Revision is 0: nothing has been
	// written, so there is no time at which it was.
	UpdatedAt time.Time
}
