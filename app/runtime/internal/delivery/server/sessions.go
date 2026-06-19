package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// wireSessionErr maps the session domain's not-found sentinel onto the wire
// sentinel, passing every other error through unchanged.
func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	return err
}

// defaultSessionPageLimit caps a single sessions.list page when the client
// gives no (or an over-large) limit.
const defaultSessionPageLimit = 100

// ListSessions paginates over the in-process session.Service with the
// same opaque-cursor mechanics as items.list (see pageByID): a non-empty
// NextCursor is the "has more" signal — never a silent truncation. The
// store returns the full ordered list; pagination is applied here.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	sessions, err := s.rt.Session().List(ctx)
	if err != nil {
		return nil, err
	}
	page, next := pageByID(sessions, func(ses session.Session) string { return ses.ID }, q.Cursor, q.Limit, defaultSessionPageLimit)
	running := s.runningSessionSet()
	waiting := s.waitingSessionSet(ctx)
	data := make([]protocol.Session, 0, len(page))
	for _, ses := range page {
		data = append(data, s.sessionToWire(ses, sessionStatus(running[ses.ID], waiting[ses.ID])))
	}
	return &protocol.Page[protocol.Session]{Data: data, NextCursor: next}, nil
}

func (s *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	ses, err := s.rt.Session().Get(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &out, nil
}

func (s *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// cwd defaults to the serve directory (ServerInfo.cwd) when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := in.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	ses, err := s.rt.Session().Create(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	// A freshly created session has no run and no interrupt — idle.
	out := s.sessionToWire(ses, protocol.SessionStatusIdle)
	return &out, nil
}

func (s *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	if err := s.rt.Session().Delete(ctx, id); err != nil {
		return wireSessionErr(err)
	}
	// Cascade: a deleted session must take its session-scoped data with it.
	// Without this the sessions row is gone but the transcript / chat-memory /
	// open interrupts still resolve for that id (items.list, runs.resume, …),
	// leaving orphans that bloat storage and surface dangling in the UI. Run
	// AFTER the authoritative session delete and best-effort: a partial cascade
	// leaves harmless orphans, never a half-deleted session. (Process snapshots
	// aren't dropped here — a finished turn discards its own at teardown,
	// [chatProcess.Discard]; a still-parked one rides its interrupt, dropped
	// below.)
	_ = s.rt.Transcript().DeleteSession(ctx, id) // history runs + items
	_ = s.rt.TruncateMessages(ctx, id, 0)        // chat-memory messages (keepN=0 clears)
	s.dropSessionInterrupts(ctx, id)             // durable open interrupts
	s.dropCheckpoints(id)                        // file snapshots (shadow git)
	return nil
}

// dropSessionInterrupts removes every open-interrupt record for a session —
// the cascade arm for runs.listOpenInterrupts. Best-effort: a failed list or
// delete leaves a resumable record that can be cleared on a later pass.
func (s *Server) dropSessionInterrupts(ctx context.Context, sessionID string) {
	pending, err := s.rt.Interrupts().List(ctx, sessionID)
	if err != nil {
		return
	}
	for _, p := range pending {
		_ = s.rt.Interrupts().Delete(ctx, p.ParentRunID)
	}
}

// UpdateSession applies a sessions.update edit: title (rename), model,
// cwd (relocate, gated by features.relocate) and metadata (full replace) are
// all live. Nil fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return nil, fmt.Errorf("%w: title must not be empty", protocol.ErrInvalidParams)
		}
		if err := s.rt.Session().Rename(ctx, in.SessionID, title); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Model != nil {
		if err := s.rt.Session().SetModel(ctx, in.SessionID, *in.Model); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Cwd != nil {
		// Relocate to a real, existing directory — a stale path would silently
		// break every later run's tool/memory resolution, so reject it now.
		info, err := os.Stat(*in.Cwd)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: %s", protocol.ErrCwdUnavailable, *in.Cwd)
		}
		if err := s.rt.Session().SetCwd(ctx, in.SessionID, *in.Cwd); err != nil {
			return nil, wireSessionErr(err)
		}
	}
	if in.Metadata != nil {
		if err := s.rt.Session().SetMetadata(ctx, in.SessionID, *in.Metadata); err != nil {
			return nil, wireSessionErr(err)
		}
	}

	ses, err := s.rt.Session().Get(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := s.sessionToWire(ses, s.liveStatus(ctx, ses.ID))
	return &out, nil
}

// ForkSession branches a session into a fresh child that continues from the
// parent's conversation (API.md §7.2 / AUX_API §4.2): the child inherits the
// parent's cwd and a copy of its chat history, then diverges. An optional title
// overrides the default "<parent> (fork)".
//
// FromRunID (run-boundary fork — "branch from this run", B4) truncate-copies
// history up to and INCLUDING that run's turn; omit it for a whole-conversation
// fork. Snapshot semantics: only completed runs are copied, so an in-flight run
// at the boundary contributes only what it has already flushed. Forking deletes
// nothing, so unlike rollback it needs no session_busy guard.
func (s *Server) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	// Resolve the copy boundary against the parent BEFORE creating the child.
	// copyN < 0 means "copy the whole history".
	copyN := -1
	if in.FromRunID != "" {
		_, runs, err := s.rt.Transcript().List(ctx, in.SessionID)
		if err != nil {
			return nil, wireSessionErr(err)
		}
		nodes, _, err := runNodes(runs)
		if err != nil {
			return nil, err
		}
		// requireRoot=false: fork is lax about the boundary run's kind (the
		// contract lists only session_not_found / run_not_found).
		b, err := transcript.BoundaryAt(nodes, in.FromRunID, false)
		if err != nil {
			return nil, wireBoundaryErr(err)
		}
		copyN = b.KeepMark // -1 (unknown watermark) falls back to a full copy below
	}

	// Fork records the branch lineage (parent id, inherited cwd); atMessageID
	// is empty because lineage is run-based now, not message-based.
	child, err := s.rt.Session().Fork(ctx, in.SessionID, "")
	if err != nil {
		return nil, wireSessionErr(err)
	}

	// Copy the parent's history prefix into the fresh child so its next turn
	// continues with the same context. The child was just created (empty), so
	// the append-only seed can't double up.
	msgs, err := s.rt.ReadHistory(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	if copyN >= 0 && copyN < len(msgs) {
		msgs = msgs[:copyN]
	}
	if err := s.rt.SeedHistory(ctx, child.ID, msgs); err != nil {
		return nil, err
	}

	if in.Title != "" {
		if err := s.rt.Session().Rename(ctx, child.ID, in.Title); err != nil {
			return nil, wireSessionErr(err)
		}
		child.Title = in.Title
	}

	// A freshly forked child has no run of its own yet — idle.
	out := s.sessionToWire(child, protocol.SessionStatusIdle)
	return &out, nil
}

// sessionToWire converts the internal session shape into the wire shape.
// Status is supplied by the caller (see liveStatus / sessionStatus) so the
// list path can batch the lookups instead of querying per session. Model falls
// back to the runtime default when the session never explicitly selected one,
// so the wire always carries a real model name (the frontend resolves the
// assistant's displayName from it).
func (s *Server) sessionToWire(ses session.Session, status protocol.SessionStatus) protocol.Session {
	meta := ses.Metadata
	if meta == nil {
		meta = map[string]any{} // Session.metadata is an object, never null (API.md §4.1)
	}
	return protocol.Session{
		ID:        ses.ID,
		Title:     ses.Title,
		Cwd:       ses.Cwd,
		Model:     ses.EffectiveModel(s.rt.DefaultModel()),
		Status:    status,
		CreatedAt: ses.StartedAt,
		UpdatedAt: ses.UpdatedAt,
		Metadata:  meta,
	}
}

// sessionStatus picks the wire status from the two live signals: running wins
// (an active run is the loudest state), then waiting (an open HITL interrupt),
// else idle.
func sessionStatus(running, waiting bool) protocol.SessionStatus {
	switch {
	case running:
		return protocol.SessionStatusRunning
	case waiting:
		return protocol.SessionStatusWaiting
	default:
		return protocol.SessionStatusIdle
	}
}

// liveStatus derives one session's status — running from the in-memory run
// registry, waiting from a targeted open-interrupt lookup. For the list path
// use the batched runningSessionSet / waitingSessionSet instead (this would be
// an N+1 there).
func (s *Server) liveStatus(ctx context.Context, sessionID string) protocol.SessionStatus {
	if s.hasActiveRun(sessionID) {
		return protocol.SessionStatusRunning
	}
	waiting := false
	if pending, err := s.rt.Interrupts().List(ctx, sessionID); err == nil {
		waiting = len(pending) > 0
	}
	return sessionStatus(false, waiting)
}

// runningSessionSet snapshots the session ids with a live run, in one lock pass
// — the list path's batched form of hasActiveRun (rollback.go).
func (s *Server) runningSessionSet() map[string]bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	set := make(map[string]bool, len(s.runs))
	for _, e := range s.runs {
		set[e.sessionID] = true
	}
	return set
}

// waitingSessionSet fetches every open interrupt once and returns the set of
// sessions awaiting a HITL answer — the list path's batched form, so per-session
// status costs no extra query. Empty on error (status degrades to running/idle).
func (s *Server) waitingSessionSet(ctx context.Context) map[string]bool {
	pending, err := s.rt.Interrupts().List(ctx, "")
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(pending))
	for _, p := range pending {
		set[p.SessionID] = true
	}
	return set
}
