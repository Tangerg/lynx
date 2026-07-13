package sessions

import (
	"context"

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

// ResolveForkHistoryPrefix applies the fork boundary to a parent history. Fork
// accepts continuation runs (requireRoot=false) and an unknown watermark falls
// back to a full-history copy, matching the existing snapshot semantics.
func ResolveForkHistoryPrefix(msgs []chat.Message, nodes []transcript.RunNode, fromRunID string) ([]chat.Message, error) {
	if fromRunID == "" {
		return msgs, nil
	}
	b, err := transcript.Timeline(nodes).BoundaryAt(fromRunID, false)
	if err != nil {
		return nil, err
	}
	if b.KeepMark >= 0 && b.KeepMark < len(msgs) {
		return msgs[:b.KeepMark], nil
	}
	return msgs, nil
}

// Fork creates a child session, seeds it with the resolved parent history
// prefix, and renames it as ONE atomic write-set (§8.1). The protocol adapter
// owns only wire decoding; the boundary semantics + chat history prefix live
// here (the application resolves the prefix; the adapter commits the branch).
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	msgs, err := c.s.ReadHistory(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	var nodes []transcript.RunNode
	if spec.FromRunID != "" {
		_, runs, err := c.s.Transcript().List(ctx, spec.ParentID)
		if err != nil {
			return session.Session{}, err
		}
		nodes = transcript.TimelineFromRuns(runs)
	}
	msgs, err = ResolveForkHistoryPrefix(msgs, nodes, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}
	return c.s.ApplyFork(ctx, ForkPlan{
		ParentID: spec.ParentID,
		Messages: msgs,
		Title:    spec.Title,
	})
}
