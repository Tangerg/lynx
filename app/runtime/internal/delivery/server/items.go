package server

import (
	"context"
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
		Page: *protocol.NewPageWithCursor(items, page.NextCursor),
		Runs: runs,
	}, nil
}
