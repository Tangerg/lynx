package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// wireSessionErr maps the session domain's not-found sentinel onto the wire
// sentinel, passing every other error through unchanged.
func wireSessionErr(err error) error {
	if errors.Is(err, session.ErrNotFound) {
		return protocol.ErrSessionNotFound
	}
	if errors.Is(err, session.ErrTitleRequired) {
		return fmt.Errorf("%w: title must not be empty", protocol.ErrInvalidParams)
	}
	if errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		return fmt.Errorf("%w: %w", protocol.ErrWorkspaceUnavailable, err)
	}
	if errors.Is(err, session.ErrRevisionConflict) {
		return fmt.Errorf("%w: the session changed after it was read", protocol.ErrRevisionConflict)
	}
	return err
}

// ListSessions returns one page of the session list (sessions.list). A non-empty
// NextCursor is the "has more" signal — never a silent truncation. The page is
// cut by the read, so only the sessions on it are resolved.
func (s *Server) ListSessions(ctx context.Context, q protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
	page, err := s.sessions.ListViewPage(ctx, q.Cursor, q.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	data := make([]protocol.Session, 0, len(page.Rows))
	for _, view := range page.Rows {
		data = append(data, presentSession(view))
	}
	return protocol.NewPageWithCursor(data, page.NextCursor), nil
}

func (s *Server) GetSession(ctx context.Context, id string) (*protocol.Session, error) {
	view, err := s.sessions.View(ctx, id)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	out := presentSession(view)
	return &out, nil
}

// GetSessionSnapshot projects one Application-owned storage snapshot into the
// complete material read used by mounted clients. Capability checks apply to
// the snapshot as a whole, so no response can silently trim a waiting set or a
// child Run that the caller could not fold.
func (s *Server) GetSessionSnapshot(ctx context.Context, in protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
	snapshot, err := s.sessions.MaterialSnapshot(ctx, in.SessionID)
	if err != nil {
		return nil, wireSessionErr(err)
	}
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	for _, pending := range snapshot.Interrupts {
		if gap := pending.Capabilities.MissingFrom(caller); !gap.IsEmpty() {
			return nil, capabilityGap(gap)
		}
	}
	out := &protocol.SessionSnapshot{
		Items:      make([]protocol.Item, 0, len(snapshot.Items)),
		Runs:       make([]protocol.RunRef, 0, len(snapshot.Runs)),
		Interrupts: make([]protocol.PendingInterruptSet, 0, len(snapshot.Interrupts)),
	}
	for _, item := range snapshot.Items {
		out.Items = append(out.Items, presentItem(item))
	}
	for _, record := range snapshot.Runs {
		if !in.IncludeDescendants && record.Lineage().IsChild() {
			continue
		}
		out.Runs = append(out.Runs, presentRun(record))
	}
	for _, pending := range snapshot.Interrupts {
		out.Interrupts = append(out.Interrupts, protocol.PendingInterruptSet{
			RootRunID:  pending.RootRunID,
			SessionID:  pending.SessionID,
			Interrupts: presentInterrupts(pending.Interrupts),
			CreatedAt:  pending.CreatedAt,
		})
	}
	if s.features.plan {
		plan := presentStoredPlan(in.SessionID, snapshot.Plan)
		out.Plan = &plan
	}
	if s.features.goals && snapshot.Goal != nil {
		out.Goal, err = presentGoal(*snapshot.Goal)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) CreateSession(ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
	// Workspace defaults to the serve directory when the
	// client omits it — cold-start zero friction (API.md §7.2 / §0.2).
	cwd := s.serverInfo.DefaultWorkspace.Path
	if in.Workspace != nil {
		cwd = in.Workspace.Path
	}
	view, err := s.sessions.CreateView(ctx, in.Title, cwd)
	if err != nil {
		return nil, err
	}
	out := presentSession(view)
	return &out, nil
}

func (s *Server) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return protocol.ErrSessionNotFound
	}
	// The session use case claims the addressed session and every owned
	// internal-subtask descendant before deleting their durable state atomically.
	// User-created forks remain independent conversations.
	if err := s.sessions.DeleteSession(ctx, id); err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return fmt.Errorf("%w: session %q or its subtask tree has a run in flight", protocol.ErrSessionBusy, id)
		}
		return wireSessionErr(err)
	}
	return nil
}

// UpdateSession applies a sessions.update edit: title (rename), model, cwd
// (relocate, gated by features.relocate), and favorite are all live. Nil
// fields are left alone; the updated session is returned. The
// dispatch layer already rejects an empty SessionID.
func (s *Server) UpdateSession(ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
	var cwd *string
	if in.Workspace != nil {
		path := in.Workspace.Path
		cwd = &path
	}
	view, err := s.sessions.UpdateView(ctx, in.SessionID, session.Patch{
		Title:            in.Title,
		Model:            in.Model,
		CWD:              cwd,
		Favorite:         in.Favorite,
		ExpectedRevision: in.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, sessions.ErrSessionBusy) {
			return nil, fmt.Errorf("%w: session %q has a run in flight", protocol.ErrSessionBusy, in.SessionID)
		}
		return nil, wireSessionErr(err)
	}
	out := presentSession(view)
	return &out, nil
}

// ForkSession branches a session into a fresh child that continues from the
// parent's conversation (API.md §7.2 / AUX_API §4.2): the child inherits the
// parent's cwd and a copy of its chat history, then diverges. An optional title
// overrides the default "<parent> (fork)".
//
// FromRunID (run-boundary fork — "branch from this run", B4) truncate-copies
// history up to and including that Run; omit it for a whole-conversation
// fork. Snapshot semantics: only terminal runs are copied; an in-flight run and
// all of its mutable history tail are excluded. Forking deletes nothing, so
// unlike rollback it needs no session_busy guard.
func (s *Server) ForkSession(ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
	child, err := s.sessions.ForkView(ctx, sessions.ForkSpec{
		ParentID:  in.SessionID,
		FromRunID: in.FromRunID,
		Title:     in.Title,
	})
	if err != nil {
		if in.FromRunID != "" {
			err = wireBoundaryErr(err)
		}
		return nil, wireSessionErr(err)
	}
	out := presentSession(child)
	return &out, nil
}

// presentSession projects the complete Application read model into the
// selected protocol shape. It intentionally performs no filesystem, live-run,
// or model-default lookup.
func presentSession(view sessions.View) protocol.Session {
	return protocol.Session{
		ID:        view.ID,
		Title:     view.Title,
		Workspace: presentWorkspaceInfo(view.CWD, view.ProjectRoot, view.CWDMissing),
		Model:     view.Model,
		Status:    presentSessionStatus(view.Activity),
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
		Favorite:  view.Favorite,
		Revision:  view.Revision,
	}
}

// presentSessionStatus projects the application-resolved activity without
// reproducing its running/waiting precedence in Delivery.
func presentSessionStatus(activity sessions.Activity) protocol.SessionStatus {
	switch activity {
	case sessions.ActivityRunning:
		return protocol.SessionStatusRunning
	case sessions.ActivityWaiting:
		return protocol.SessionStatusWaiting
	default:
		return protocol.SessionStatusIdle
	}
}
