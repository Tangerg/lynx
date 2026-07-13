package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

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
// text, createdAt).
func (s *Server) ListItems(ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	hItems, hRuns, err := s.queries.ListTranscript(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	return s.listItemsFromHistory(hItems, hRuns, in)
}

// defaultItemPageLimit caps a single items.list page when the client gives
// no (or an over-large) limit.
const defaultItemPageLimit = 200

// pageByID applies opaque-cursor + limit pagination over an ordered slice
// whose elements carry a unique id. cursor is the previous page's last id
// (opaque to the client); a non-empty returned cursor is the "has more"
// signal (§4.11) — the server never silently truncates. An unknown cursor
// yields an empty page (the referenced element is gone), which the client
// reads as "no more". Shared by items.list and sessions.list so both
// surfaces keep identical cursor mechanics.
func pageByID[T any](elems []T, id func(T) string, cursor string, limit, maxLimit int) ([]T, string) {
	if cursor != "" {
		start := len(elems) // unknown cursor → past the end → empty page
		for idx, el := range elems {
			if id(el) == cursor {
				start = idx + 1
				break
			}
		}
		elems = elems[start:]
	}
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	if len(elems) > limit {
		page := elems[:limit]
		return page, id(page[len(page)-1])
	}
	return elems, ""
}

// listItemsFromHistory serves items.list from durable Item rows.
func (s *Server) listItemsFromHistory(hItems []transcript.Item, hRuns []transcript.Run, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	pageRows, next := pageByID(hItems, func(item transcript.Item) string { return item.ID }, in.Cursor, in.Limit, defaultItemPageLimit)
	items := make([]protocol.Item, 0, len(pageRows))
	for _, item := range pageRows {
		items = append(items, presentItem(item))
	}
	// Runs stay fully decoded: the client needs the whole run tree to thread
	// items, and the per-session run count is small.
	runs := make([]protocol.RunRef, 0, len(hRuns))
	for _, run := range hRuns {
		runs = append(runs, presentRun(run))
	}

	return &protocol.ListItemsResponse{
		Page: protocol.Page[protocol.Item]{Data: items, NextCursor: next},
		Runs: runs,
	}, nil
}
