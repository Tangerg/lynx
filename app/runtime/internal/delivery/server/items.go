package server

import (
	"context"
	"encoding/base64"
	"fmt"

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

// pageByCursor applies opaque-cursor + limit pagination over an ordered slice
// whose elements carry a unique stable key. cursor is the previous page's key
// (opaque to the client); a non-empty returned cursor is the "has more"
// signal (§4.11) — the server never silently truncates. A cursor whose anchor
// no longer exists is rejected instead of guessing against a collection whose
// ordering may not match its identity key. The client then restarts from page
// one, avoiding silent skips or duplicates.
func pageByCursor[T any](elems []T, key func(T) string, cursor string, limit, maxLimit int) ([]T, string, error) {
	if limit < 0 {
		return nil, "", fmt.Errorf("%w: limit must not be negative", protocol.ErrInvalidParams)
	}
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) == 0 {
			return nil, "", fmt.Errorf("%w: cursor is invalid", protocol.ErrInvalidParams)
		}
		start := -1
		for idx, el := range elems {
			if key(el) == string(decoded) {
				start = idx + 1
				break
			}
		}
		if start < 0 {
			return nil, "", fmt.Errorf("%w: cursor anchor is no longer available", protocol.ErrInvalidParams)
		}
		elems = elems[start:]
	}
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	if len(elems) > limit {
		page := elems[:limit]
		return page, base64.RawURLEncoding.EncodeToString([]byte(key(page[len(page)-1]))), nil
	}
	return elems, "", nil
}

// pageOrderedByCursor is the deletion-tolerant variant for collections that
// are explicitly sorted by the same lexical key stored in the cursor. It can
// safely seek to the first greater key when the anchor disappears.
func pageOrderedByCursor[T any](elems []T, key func(T) string, cursor string, limit, maxLimit int) ([]T, string, error) {
	if cursor == "" {
		return pageByCursor(elems, key, cursor, limit, maxLimit)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(decoded) == 0 {
		return nil, "", fmt.Errorf("%w: cursor is invalid", protocol.ErrInvalidParams)
	}
	anchor := string(decoded)
	start := len(elems)
	for idx, elem := range elems {
		if candidate := key(elem); candidate >= anchor {
			start = idx
			if candidate == anchor {
				start++
			}
			break
		}
	}
	return pageByCursor(elems[start:], key, "", limit, maxLimit)
}

// listItemsFromHistory serves items.list from durable Item rows.
func (s *Server) listItemsFromHistory(hItems []transcript.Item, hRuns []transcript.Run, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	pageRows, next, err := pageByCursor(hItems, func(item transcript.Item) string { return item.ID }, in.Cursor, in.Limit, defaultItemPageLimit)
	if err != nil {
		return nil, err
	}
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
