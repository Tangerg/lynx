package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
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
	return s.listItemsFromHistory(ctx, s.rt.History(), in)
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

// listItemsFromHistory serves items.list from the durable Item store.
func (s *Server) listItemsFromHistory(ctx context.Context, store history.Store, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	hItems, hRuns, err := store.List(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	// Page first, decode after: pagination is a row-level operation and
	// the item id is a plain column, so only the returned page's blobs
	// are unmarshaled (a long session would otherwise decode thousands
	// of items to serve 200). A corrupt row occupies its page slot and
	// is skipped at decode — same tolerance as a cursor pointing at a
	// deleted element.
	pageRows, next := pageByID(hItems, func(hi history.Item) string { return hi.ItemID }, in.Cursor, in.Limit, defaultItemPageLimit)
	items := make([]protocol.Item, 0, len(pageRows))
	for _, hi := range pageRows {
		var it protocol.Item
		if err := json.Unmarshal(hi.Blob, &it); err != nil {
			continue // skip a corrupt row rather than failing the whole list
		}
		items = append(items, it)
	}
	// Runs stay fully decoded: the client needs the whole run tree to
	// thread items, the per-session run count is small, and
	// reconcileLostRun must inspect each ref.
	runs := make([]protocol.RunRef, 0, len(hRuns))
	for _, hr := range hRuns {
		var r protocol.RunRef
		if err := json.Unmarshal(hr.Blob, &r); err != nil {
			continue
		}
		s.reconcileLostRun(&r)
		runs = append(runs, r)
	}

	return &protocol.ListItemsResponse{
		Page: protocol.Page[protocol.Item]{Data: items, NextCursor: next},
		Runs: runs,
	}, nil
}

// reconcileLostRun heals a RunRef the durable history left at status:running
// when no live pump is driving it: such a run was lost to a restart/crash
// between run.started and its terminal run.finished. Nothing is advancing it
// and it isn't resumable (no interrupt was recorded — a parked run finishes
// with outcome:interrupt, which IS terminal in history), so without this the
// client renders a perpetual spinner. We present it as a terminal
// error(run_lost) (API.md §6.2 anti-dangling, applied to non-parked runs).
//
// Reconciliation is in-memory on the read path, not a write-back: it re-judges
// liveness from the run table on every items.list, so a run is never wrongly
// terminalized (the table entry is set before the first persist and cleared
// only after the terminal one — a genuinely live run is always present). No-op
// for already-terminal runs.
func (s *Server) reconcileLostRun(r *protocol.RunRef) {
	if r.Status != protocol.RunStatusRunning || s.isRunLive(r.ID) {
		return
	}
	r.Status = protocol.RunStatusFinished
	r.Outcome = &protocol.RunOutcome{
		Type:   protocol.OutcomeError,
		Result: &protocol.RunResult{Error: &protocol.ProblemData{Type: "run_lost", Channel: "run", Detail: "run lost on restart"}},
	}
	if r.FinishedAt.IsZero() {
		r.FinishedAt = time.Now().UTC()
	}
}

// isRunLive reports whether a run is currently being pumped in this process.
func (s *Server) isRunLive(runID string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	_, ok := s.runs[runID]
	return ok
}
