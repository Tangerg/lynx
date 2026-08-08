package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// wirePageError maps a read's page-request rejection onto invalid_params. A
// cursor the read will not continue is a bad request, not a runtime failure:
// letting it fall through to the unrecognized-error default would tell the client
// the server broke and hide the one remedy — start from the first page.
func wirePageError(err error) error {
	if errors.Is(err, keyset.ErrInvalidCursor) || errors.Is(err, keyset.ErrInvalidLimit) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

// ListItems returns a session's persisted history as durable Items
// (API.md §7.4). History = the completed Item sequence; there is no
// separate Message type. The result is a Page[Item] (`data` + `nextCursor`)
// plus the RunRefs needed to rebuild the run tree (§10.3). Over a page the
// server backfills nextCursor rather than silently truncating (§4.11 — no
// silent caps); a returned cursor is the opaque "has more" token the client
// passes back to continue.
//
// The source is the durable Item-history store (a required runtime
// dependency): the exact Items the runtime streamed (same ids, runId,
// text, createdAt). The page is cut by the query, so a long session's
// history is not loaded to return a slice of it.
func (s *Server) ListItems(ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	scope, err := itemScopeFromWire(in.Scope)
	if err != nil {
		return nil, err
	}
	if in.Scope.Type == protocol.ItemScopeRun {
		run, found, err := s.queries.Run(ctx, in.Scope.RunID)
		switch {
		case err != nil:
			return nil, err
		case !found:
			return nil, protocol.ErrRunNotFound
		case run.Lineage().IsChild():
			if err := s.requireFeature(ctx, protocol.FeatureSubagents); err != nil {
				return nil, err
			}
		}
	}
	order, err := sequenceOrderFromWire(in.Order)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListItemPage(ctx, scope, order, in.Cursor, in.Limit)
	if err != nil {
		return nil, wireItemScopeError(wirePageError(err))
	}
	items := make([]protocol.Item, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, presentItem(item))
	}
	runs := make([]protocol.RunSummary, 0, len(page.Runs))
	for _, run := range page.Runs {
		runs = append(runs, presentRunSummary(run))
	}
	return &protocol.ListItemsResponse{
		Page: *protocol.NewPageWithCursor(items, page.NextCursor),
		Runs: runs,
	}, nil
}

// GetPlan returns the session's Plan projection — the same shape the run
// stream publishes, so a client folding the stream and a client that just asked are
// holding one value and not two descriptions of it.
//
// A session with no list yet answers with the empty state at revision 0. That is a
// fact rather than a gap: the panel renders empty, and only a session that does not
// exist is an error.
func (s *Server) GetPlan(ctx context.Context, in protocol.GetPlanRequest) (*protocol.StateSnapshot, error) {
	state, err := s.queries.PlanState(ctx, in.SessionID)
	if err != nil {
		return nil, wireItemScopeError(err)
	}
	snapshot := presentPlanState(in.SessionID, state)
	return &snapshot, nil
}

// itemScopeFromWire reads the scope union. The tag decides which fields are read —
// that is what a discriminated union means — so a field belonging to the other
// variant is left alone rather than blended in: the schema states the exclusivity
// for clients, and the server that honored both would be answering a request no
// valid client sends.
//
// A run scope is legal for a root or a child run, and it carries no sessionId: the
// run's own record says where it lives.
func itemScopeFromWire(scope protocol.ItemListScope) (queries.ItemScope, error) {
	switch scope.Type {
	case protocol.ItemScopeSession:
		if scope.SessionID == "" {
			return queries.ItemScope{}, fmt.Errorf("%w: scope.sessionId is required for a session scope", protocol.ErrInvalidParams)
		}
		return queries.SessionItems(scope.SessionID), nil
	case protocol.ItemScopeRun:
		if scope.RunID == "" {
			return queries.ItemScope{}, fmt.Errorf("%w: scope.runId is required for a run scope", protocol.ErrInvalidParams)
		}
		if scope.IncludeDescendants {
			return queries.RunTreeItems(scope.RunID), nil
		}
		return queries.RunItems(scope.RunID), nil
	default:
		return queries.ItemScope{}, fmt.Errorf("%w: scope.type must be %q or %q", protocol.ErrInvalidParams,
			protocol.ItemScopeSession, protocol.ItemScopeRun)
	}
}

// sequenceOrderFromWire reads the page direction, defaulting to oldest first: a
// caller that says nothing gets the order the runtime produced, which is the one a
// reducer can fold.
func sequenceOrderFromWire(order protocol.ItemOrder) (transcript.SequenceOrder, error) {
	switch order {
	case "", protocol.ItemOrderAsc:
		return transcript.OldestFirst, nil
	case protocol.ItemOrderDesc:
		return transcript.NewestFirst, nil
	default:
		return "", fmt.Errorf("%w: unknown order %q", protocol.ErrInvalidParams, order)
	}
}

// wireItemScopeError maps a scope that names nothing onto the wire's own words. The
// read refuses it rather than answering with an empty page, and the two subjects get
// two errors because the client's next move differs: find the session, or find the
// run.
func wireItemScopeError(err error) error {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	default:
		return err
	}
}
