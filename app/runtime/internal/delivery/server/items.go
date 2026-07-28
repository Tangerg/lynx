package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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
	page, err := s.queries.ListItemPage(ctx, in.SessionID, in.Cursor, in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	items := make([]protocol.Item, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, presentItem(item))
	}
	runs := make([]protocol.RunRef, 0, len(page.Runs))
	for _, run := range page.Runs {
		runs = append(runs, presentRun(run))
	}
	return &protocol.ListItemsResponse{
		Page: protocol.Page[protocol.Item]{Data: items, NextCursor: page.NextCursor},
		Runs: runs,
	}, nil
}

// pageByCursor is the in-memory pager the remaining list handlers still use while
// their reads move behind keyset queries. It goes away with its last caller.
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
