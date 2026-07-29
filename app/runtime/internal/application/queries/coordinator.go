// Package queries is the application-owned read surface over a session's durable
// execution record: the transcript (items + runs) and open HITL interrupts.
// These are projections read directly from persistence (§5.4) — no aggregate is
// loaded and no command store is fattened with reads. Delivery drives them for
// runs.list, items.list and interrupts.list.
package queries

import (
	"context"
	"strconv"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// The query a page cursor belongs to, so a cursor minted by another read is
// rejected instead of continuing this one. Each is the name of the PUBLISHED method
// the read serves: the namespace only has to be unique, but spelling it any other
// way gives one query two names, and then nothing can check that a reader binds its
// cursors to its own query. [arch.TestPageCursorsBindToTheirOwnMethod] holds the
// convention.
const (
	itemPageMethod      = "items.list"
	runPageMethod       = "runs.list"
	interruptPageMethod = "runs.listOpenInterrupts"
)

// Page ceilings, per read. A client asking for more gets this many and a cursor.
const (
	itemPageLimit      = 200
	runPageLimit       = 100
	interruptPageLimit = 100
)

// TranscriptReader is the coordinator's view of the durable item history. Items
// arrive one bounded page at a time, seeking past the previous page's position.
type TranscriptReader interface {
	PageItems(ctx context.Context, sessionID string, afterSequence int64, limit int) ([]transcript.SequencedItem, error)
}

// InterruptReader is the coordinator's view of the open-interrupt registry: a
// session's open interrupts, or every pending interrupt when sessionID is empty.
type InterruptReader interface {
	ListPage(ctx context.Context, sessionID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error)
}

// RunReader is the coordinator's view of the durable Run record: the Runs that
// are executing, in one session or across all of them, and a session's Runs in
// full. The latter arrive whole rather than paged, because threading an item to
// its Run needs the tree and a session holds few of them.
type RunReader interface {
	ListRunning(ctx context.Context, sessionID string, afterStartedAt int64, afterRunID string, limit int) ([]transcript.Run, error)
	ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

// Coordinator serves the session read projections. Stateless beyond its store
// collaborators; safe to share.
type Coordinator struct {
	transcript TranscriptReader
	interrupts InterruptReader
	runs       RunReader
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Transcript TranscriptReader
	Interrupts InterruptReader
	Runs       RunReader
}

// New returns a query Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		transcript: deps.Transcript,
		interrupts: deps.Interrupts,
		runs:       deps.Runs,
	}
}

// ItemPage is one page of a session's history, with the run tree needed to thread
// the items on it.
type ItemPage struct {
	Items      []transcript.Item
	NextCursor string
	Runs       []transcript.Run
}

// ListItemPage returns one page of a session's durable history, continuing after
// cursor. A page is bounded in the query: the previous page's position is the
// seek anchor, so serving the tail of a long session costs a page, not the whole
// timeline.
//
// An unusable cursor is refused rather than reinterpreted — see
// [keyset.ErrInvalidCursor]. Silently restarting from the top would look like a
// page of duplicates to a client that had already read them.
func (c *Coordinator) ListItemPage(ctx context.Context, sessionID, cursor string, limit int) (ItemPage, error) {
	filters := []string{sessionID}
	anchor, err := keyset.Decode(cursor, itemPageMethod, filters)
	if err != nil {
		return ItemPage{}, err
	}
	afterSequence, err := sequenceAnchor(anchor)
	if err != nil {
		return ItemPage{}, err
	}
	size, err := keyset.Limit(limit, itemPageLimit)
	if err != nil {
		return ItemPage{}, err
	}

	// One row past the page: having it is how "there is more" is known without a
	// second count, and it is dropped before the page is returned.
	sequenced, err := c.transcript.PageItems(ctx, sessionID, afterSequence, size+1)
	if err != nil {
		return ItemPage{}, err
	}
	page := keyset.PageOf(sequenced, size, itemPageMethod, filters,
		func(entry transcript.SequencedItem) []string {
			return []string{strconv.FormatInt(entry.Sequence, 10)}
		})

	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return ItemPage{}, err
	}
	items := make([]transcript.Item, 0, len(page.Rows))
	for _, entry := range page.Rows {
		items = append(items, entry.Item)
	}
	return ItemPage{Items: items, NextCursor: page.NextCursor, Runs: runs}, nil
}

// sequenceAnchor reads a decoded cursor's sort position. A token whose key is not
// a sequence was not minted by this read, whatever else about it matched.
func sequenceAnchor(anchor []string) (int64, error) {
	if len(anchor) == 0 {
		return 0, nil
	}
	sequence, err := strconv.ParseInt(anchor[0], 10, 64)
	if err != nil || len(anchor) != 1 {
		return 0, keyset.ErrInvalidCursor
	}
	return sequence, nil
}

// ListRunningRuns returns one page of the Runs currently executing, scoped to
// sessionID when it is non-empty and continuing after cursor. It reads the
// durable admission record rather than a live in-process registry: the registry
// only knows the segments THIS process is streaming, so it answers a different
// question than the one being asked, and answers it differently after a restart.
func (c *Coordinator) ListRunningRuns(ctx context.Context, sessionID, cursor string, limit int) (keyset.Page[transcript.Run], error) {
	filters := []string{sessionID}
	afterStartedAt, afterID, err := timeAndIDAnchor(cursor, runPageMethod, filters)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	size, err := keyset.Limit(limit, runPageLimit)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	rows, err := c.runs.ListRunning(ctx, sessionID, afterStartedAt, afterID, size+1)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	return keyset.PageOf(rows, size, runPageMethod, filters, func(run transcript.Run) []string {
		return []string{strconv.FormatInt(run.CreatedAt.UnixNano(), 10), run.ID}
	}), nil
}

// ListPendingInterruptPage returns one page of the durable open HITL interrupts,
// oldest first, continuing after cursor. An empty sessionID pages across every
// session.
func (c *Coordinator) ListPendingInterruptPage(ctx context.Context, sessionID, cursor string, limit int) (keyset.Page[interrupts.Pending], error) {
	filters := []string{sessionID}
	afterCreatedAt, afterID, err := timeAndIDAnchor(cursor, interruptPageMethod, filters)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	size, err := keyset.Limit(limit, interruptPageLimit)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	rows, err := c.interrupts.ListPage(ctx, sessionID, afterCreatedAt, afterID, size+1)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	return keyset.PageOf(rows, size, interruptPageMethod, filters, func(pending interrupts.Pending) []string {
		return []string{strconv.FormatInt(pending.CreatedAt.UnixNano(), 10), pending.RunID}
	}), nil
}

// timeAndIDAnchor reads a decoded cursor's (timestamp, id) sort position. The id
// is what makes the order total: two rows can share a nanosecond, and a
// timestamp-only bound would then drop one or return it twice.
func timeAndIDAnchor(cursor, method string, filters []string) (int64, string, error) {
	anchor, err := keyset.Decode(cursor, method, filters)
	if err != nil {
		return 0, "", err
	}
	if len(anchor) == 0 {
		return 0, "", nil
	}
	if len(anchor) != 2 {
		return 0, "", keyset.ErrInvalidCursor
	}
	stamp, err := strconv.ParseInt(anchor[0], 10, 64)
	if err != nil {
		return 0, "", keyset.ErrInvalidCursor
	}
	return stamp, anchor[1], nil
}
