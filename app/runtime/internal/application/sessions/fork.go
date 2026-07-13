package sessions

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ForkSpec describes where a session fork should branch.
type ForkSpec struct {
	ParentID  string
	FromRunID string
	Title     string
}

// ResolveForkHistoryPrefix applies a durable run boundary to parent history.
// Non-terminal runs never contribute messages: their current tail can still
// change and therefore is not a portable fork boundary. An explicit target
// must itself be terminal; an implicit whole-conversation fork stops at the
// latest terminal run.
func ResolveForkHistoryPrefix(msgs []chat.Message, runs []transcript.Run, fromRunID string) ([]chat.Message, error) {
	ordered := slices.Clone(runs)
	slices.SortStableFunc(ordered, func(a, b transcript.Run) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	for _, run := range ordered {
		if run.State.IsTerminal() && (run.MessageMark < 0 || run.MessageMark > len(msgs)) {
			return nil, fmt.Errorf("sessions: terminal run %q has invalid message watermark %d", run.ID, run.MessageMark)
		}
	}

	// A root run and the subagents it spawned are one turn boundary. A terminal
	// subagent inside an active root does not make that active turn portable, so
	// include a group only when every run in it is terminal.
	terminal := make([]transcript.RunNode, 0, len(ordered))
	targetTerminal := fromRunID == ""
	for start := 0; start < len(ordered); {
		if ordered[start].SpawnedByItemID != "" {
			return nil, fmt.Errorf("sessions: run timeline starts a group with subagent %q", ordered[start].ID)
		}
		end := start + 1
		for end < len(ordered) && ordered[end].SpawnedByItemID != "" {
			end++
		}
		stable := true
		for _, run := range ordered[start:end] {
			stable = stable && run.State.IsTerminal()
		}
		if stable {
			for _, run := range ordered[start:end] {
				terminal = append(terminal, transcript.RunNode{
					ID: run.ID, SpawnedByItemID: run.SpawnedByItemID,
					CreatedAt: run.CreatedAt, Mark: run.MessageMark,
				})
				if run.ID == fromRunID {
					targetTerminal = true
				}
			}
		}
		start = end
	}
	if !targetTerminal {
		return nil, transcript.ErrRunNotFound
	}
	if len(terminal) == 0 {
		return nil, nil
	}
	if fromRunID == "" {
		fromRunID = terminal[len(terminal)-1].ID
	}
	b, err := transcript.Timeline(terminal).BoundaryAt(fromRunID, false)
	if err != nil {
		return nil, err
	}
	return slices.Clone(msgs[:b.KeepMark]), nil
}

// Fork creates a child session, seeds it with the resolved parent history
// prefix, and renames it as ONE atomic write-set (§8.1). The protocol adapter
// owns only wire decoding; the boundary semantics + chat history prefix live
// here (the application resolves the prefix; the adapter commits the branch).
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	snapshot, err := c.s.ReadSnapshot(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	msgs, err := ResolveForkHistoryPrefix(snapshot.Messages, snapshot.Runs, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}
	return c.s.ApplyFork(ctx, ForkPlan{
		ParentID: spec.ParentID,
		Messages: msgs,
		Title:    spec.Title,
	})
}
