package lifecycle

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ForkSpec describes where a session fork should branch. Runs are the timeline
// nodes for ParentID; empty FromRunID means copy the whole conversation.
type ForkSpec struct {
	ParentID  string
	FromRunID string
	Runs      []transcript.RunNode
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
// prefix, and renames it as ONE transaction. The protocol adapter owns only
// wire decoding; the boundary semantics and chat history prefix live here.
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	msgs, err := c.s.ReadHistory(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	msgs, err = ResolveForkHistoryPrefix(msgs, spec.Runs, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}

	var child session.Session
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		ch, err := c.s.Session().Fork(ctx, spec.ParentID, "")
		if err != nil {
			return err
		}
		if err := c.s.SeedHistory(ctx, ch.ID, msgs); err != nil {
			return err
		}
		if spec.Title != "" {
			if err := c.s.Session().Rename(ctx, ch.ID, spec.Title); err != nil {
				return err
			}
			ch.Title = spec.Title
		}
		child = ch
		return nil
	}); err != nil {
		return session.Session{}, err
	}
	return child, nil
}
