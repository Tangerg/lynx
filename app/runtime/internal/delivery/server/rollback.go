package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
)

// RollbackSession discards the runs after the kept boundary, truncating the
// session in place at a run granularity (AUX_API §4.1). Destructive: it
// truncates the conversation message log to the kept watermark, deletes the dropped
// runs' durable items + records, clears their dangling interrupts, and purges
// the subagent child sessions they spawned. ToRunID is inclusive-keep (omit =
// clear to empty). Rejected with session_busy while a run is in flight.
func (s *Server) RollbackSession(ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	ses, err := s.sessionCatalog.SessionByID(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	admission, err := s.mutationAdmissions.ClaimMutationSlot(sessionClaimer{s: s}, in.SessionID)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, in.SessionID)
		}
		return nil, err
	}
	defer admission.Release()

	intent, err := rollbackIntentFromWire(in)
	if err != nil {
		return nil, err
	}

	// A file restore's `git reset --hard` writes the working tree, which a sibling
	// session sharing this cwd (a fork inherits the parent's cwd; two sessions can
	// open one dir) would race — and that sibling's tool writes never take the
	// checkpoint lock. The per-session guard above only covers THIS session, so
	// widen it to the whole tree for file restores. (History-only rollback touches
	// just this session's log, so the per-session guard suffices.)
	if intent.restoreFiles {
		restoreCwd := worktree.CanonicalCwd(ses.Cwd)
		treeAdmission, ok := s.mutationAdmissions.ClaimWorkingTreeMutation(restoreCwd)
		if !ok {
			return nil, fmt.Errorf("%w: working tree %q has a run admission in flight", protocol.ErrSessionBusy, ses.Cwd)
		}
		defer treeAdmission.Release()
		if busy := s.hasActiveRunSharingCwd(restoreCwd); busy != "" {
			return nil, fmt.Errorf("%w: session %q shares this working tree and has a run in flight", protocol.ErrSessionBusy, busy)
		}
	}

	items, runs, err := s.transcriptContent.ListTranscript(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	nodes, refByID, err := runBoundaryNodes(runs)
	if err != nil {
		return nil, err
	}
	b, err := lifecycle.ResolveRollbackBoundary(nodes, in.ToRunID)
	if err != nil {
		return nil, wireBoundaryErr(err)
	}

	// Files first — for "both" this is the atomicity guarantee: if the working
	// tree can't be restored, return now and leave history untouched.
	if intent.restoreFiles {
		if err := s.restoreCheckpoint(ctx, in.SessionID, in.ToRunID); err != nil {
			return nil, err
		}
	}

	if !intent.restoreHistory || len(b.Dropped) == 0 {
		// History stays (files-only rollback), or ToRunID is already the latest
		// turn so there's nothing after it to drop.
		out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
		return &protocol.RollbackSessionResponse{Session: &out, DroppedRuns: []protocol.DroppedRun{}}, nil
	}

	// The destructive write-set truncates the conversation message log to the kept
	// watermark + drops each dropped run's items/record + dangling interrupt as
	// ONE transaction (a failure can't leave a run whose messages were already
	// truncated away), then purges the subagent subtree those runs spawned.
	if err := s.sessionRollback.RollbackResolved(ctx, in.SessionID, b); err != nil {
		return nil, err
	}

	// Each dropped run reports its opening user input so the client can
	// re-populate the composer.
	userByRun := openingUserInputByRun(items)
	out := make([]protocol.DroppedRun, 0, len(b.Dropped))
	for _, rec := range b.Dropped {
		out = append(out, protocol.DroppedRun{Run: refByID[rec.ID], UserInput: userByRun[rec.ID]})
	}
	sess := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &protocol.RollbackSessionResponse{Session: &sess, DroppedRuns: out}, nil
}
