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

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// itemPageMethod names the query a page cursor belongs to, so a cursor minted by
// another read is rejected instead of continuing this one.
const itemPageMethod = "items.list"

// TranscriptReader is the coordinator's view of the durable transcript
// projection. Items arrive one bounded page at a time, seeking past the previous
// page's position; runs arrive whole, because threading an item to its run needs
// the tree and a session holds few of them.
type TranscriptReader interface {
	PageItems(ctx context.Context, sessionID string, afterSequence int64, limit int) ([]transcript.SequencedItem, error)
	ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

// InterruptReader is the coordinator's view of the open-interrupt registry: a
// session's open interrupts, or every pending interrupt when sessionID is empty.
type InterruptReader interface {
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
}

// RunReader is the coordinator's view of the durable Run admission record: the
// Runs that are executing, in one session or across all of them.
type RunReader interface {
	ListRunning(ctx context.Context, sessionID string) ([]execution.AdmittedRun, error)
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

// itemPageLimit is the widest items.list page this read will serve. A client
// asking for more gets this many and a cursor.
const itemPageLimit = 200

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

	runs, err := c.transcript.ListRuns(ctx, sessionID)
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

// ListPendingInterrupts returns durable open HITL interrupts. An empty sessionID
// returns every pending interrupt.
func (c *Coordinator) ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return c.interrupts.List(ctx, sessionID)
}

// ListRunningRuns returns the Runs currently executing, scoped to sessionID when
// it is non-empty. It reads the durable admission record rather than a live
// in-process registry: the registry only knows the segments THIS process is
// streaming, so it answers a different question than the one being asked, and
// answers it differently after a restart.
func (c *Coordinator) ListRunningRuns(ctx context.Context, sessionID string) ([]execution.AdmittedRun, error) {
	return c.runs.ListRunning(ctx, sessionID)
}
