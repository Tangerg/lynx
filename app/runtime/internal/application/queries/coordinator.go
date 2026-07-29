// Package queries is the application-owned read surface over a session's durable
// execution record: the transcript (items + runs) and open HITL interrupts.
// These are projections read directly from persistence (§5.4) — no aggregate is
// loaded and no command store is fattened with reads. Delivery drives them for
// runs.list, items.list and interrupts.list.
package queries

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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
// arrive one bounded page at a time, seeking from the previous page's position in
// the direction asked for.
//
// The two scopes are two methods rather than one method with a nullable subject:
// "exactly one of these is set" is a contract nothing checks, and each of these
// reads one thing.
type TranscriptReader interface {
	PageSessionItems(ctx context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
	PageRunItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
}

// SessionReader answers only whether a session exists. The item read needs that
// much and no more: a scope naming no session is a refusal, and an empty page would
// tell the client its session is empty instead.
type SessionReader interface {
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// InterruptReader is the coordinator's view of the open-interrupt registry: a
// session's open interrupts, or every pending interrupt when sessionID is empty.
type InterruptReader interface {
	ListPage(ctx context.Context, sessionID string, afterCreatedAt int64, afterRunID string, limit int) ([]interrupts.Pending, error)
}

// RunReader is the coordinator's view of the durable Run record: one Run by id, a
// named set of them, and a browsable page of root Runs. The set is how a page of
// items is threaded onto its Runs — the page's own Runs, not the session's, because
// a client rebuilds the tree it can see and a long session should not pay for the
// Runs it is not looking at.
type RunReader interface {
	Run(ctx context.Context, runID string) (transcript.Run, bool, error)
	RunsByID(ctx context.Context, runIDs []string) ([]transcript.Run, error)
	PageRuns(ctx context.Context, sessionID string, statuses []execution.RunStatus, beforeCreatedAt int64, beforeRunID string, limit int) ([]transcript.Run, error)
}

// Coordinator serves the session read projections. Stateless beyond its store
// collaborators; safe to share.
type Coordinator struct {
	transcript TranscriptReader
	interrupts InterruptReader
	runs       RunReader
	sessions   SessionReader
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Transcript TranscriptReader
	Interrupts InterruptReader
	Runs       RunReader
	Sessions   SessionReader
}

// New returns a query Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		transcript: deps.Transcript,
		interrupts: deps.Interrupts,
		runs:       deps.Runs,
		sessions:   deps.Sessions,
	}
}

// ItemPage is one page of a session's history, with the run tree needed to thread
// the items on it.
type ItemPage struct {
	Items      []transcript.Item
	NextCursor string
	Runs       []transcript.Run
}

// ItemScope is what an item page is a page OF: a whole session timeline, or one
// Run's own items. Exactly one is meaningful, which is why it is a closed choice
// here too and not two optional strings — a scope naming both would have to be
// resolved by a precedence rule, and a precedence rule is a place for the two to
// disagree.
type ItemScope struct {
	SessionID string
	RunID     string
}

// SessionItems scopes a page to a session's whole timeline.
func SessionItems(sessionID string) ItemScope { return ItemScope{SessionID: sessionID} }

// RunItems scopes a page to one Run's own items. The Run's session is resolved from
// the Run, so no caller has to supply both and risk supplying two different ones.
func RunItems(runID string) ItemScope { return ItemScope{RunID: runID} }

// ListItemPage returns one page of durable history within scope, in the direction
// order names, continuing from cursor. A page is bounded in the query: the previous
// page's position is the seek anchor, so serving the tail of a long session costs a
// page, not the whole timeline.
//
// A scope naming nothing that exists is refused with [session.ErrNotFound] or
// [transcript.ErrRunNotFound]. An empty page would be a worse answer to a wrong id
// than an error is: it says the session or run is empty, which is a fact about
// something that does not exist.
//
// An unusable cursor is refused rather than reinterpreted — see
// [keyset.ErrInvalidCursor]. Silently restarting from the top would look like a
// page of duplicates to a client that had already read them.
func (c *Coordinator) ListItemPage(ctx context.Context, scope ItemScope, order transcript.SequenceOrder, cursor string, limit int) (ItemPage, error) {
	filters := []string{scope.SessionID, scope.RunID, order.String()}
	anchor, err := keyset.Decode(cursor, itemPageMethod, filters)
	if err != nil {
		return ItemPage{}, err
	}
	fromSequence, err := sequenceAnchor(anchor)
	if err != nil {
		return ItemPage{}, err
	}
	size, err := keyset.Limit(limit, itemPageLimit)
	if err != nil {
		return ItemPage{}, err
	}
	if err := c.requireScope(ctx, scope); err != nil {
		return ItemPage{}, err
	}

	// One row past the page: having it is how "there is more" is known without a
	// second count, and it is dropped before the page is returned.
	sequenced, err := c.readScope(ctx, scope, order, fromSequence, size+1)
	if err != nil {
		return ItemPage{}, err
	}
	page := keyset.PageOf(sequenced, size, itemPageMethod, filters,
		func(entry transcript.SequencedItem) []string {
			return []string{strconv.FormatInt(entry.Sequence, 10)}
		})

	items := make([]transcript.Item, 0, len(page.Rows))
	for _, entry := range page.Rows {
		items = append(items, entry.Item)
	}
	runs, err := c.runs.RunsByID(ctx, referencedRuns(items))
	if err != nil {
		return ItemPage{}, err
	}
	return ItemPage{Items: items, NextCursor: page.NextCursor, Runs: runs}, nil
}

// requireScope refuses a scope whose subject does not exist. It runs after the
// cursor and limit are validated: a malformed request is malformed whether or not
// its subject exists, and answering "no such session" to a request that was never
// answerable would send the caller looking in the wrong place.
func (c *Coordinator) requireScope(ctx context.Context, scope ItemScope) error {
	if scope.RunID != "" {
		if _, found, err := c.runs.Run(ctx, scope.RunID); err != nil || !found {
			return cmp.Or(err, transcript.ErrRunNotFound)
		}
		return nil
	}
	found, err := c.sessions.Exists(ctx, scope.SessionID)
	if err != nil {
		return err
	}
	if !found {
		return session.ErrNotFound
	}
	return nil
}

func (c *Coordinator) readScope(ctx context.Context, scope ItemScope, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if scope.RunID != "" {
		return c.transcript.PageRunItems(ctx, scope.RunID, order, fromSequence, limit)
	}
	return c.transcript.PageSessionItems(ctx, scope.SessionID, order, fromSequence, limit)
}

// referencedRuns is the distinct Runs this page's items belong to, in first-seen
// order. It is what the page carries instead of the session's Run list: the client
// merges these across pages, so what it can thread is exactly what it has read.
func referencedRuns(items []transcript.Item) []string {
	var out []string
	for _, item := range items {
		if item.RunID != "" && !slices.Contains(out, item.RunID) {
			out = append(out, item.RunID)
		}
	}
	return out
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

// Run returns one Run by id, reporting false when no Run has that id. It reads
// the durable record, so a Run this process never streamed — parked, finished, or
// admitted before a restart — answers the same as one it is streaming now.
func (c *Coordinator) Run(ctx context.Context, runID string) (transcript.Run, bool, error) {
	if runID == "" {
		return transcript.Run{}, false, nil
	}
	return c.runs.Run(ctx, runID)
}

// ListRunPage returns one page of the root Runs matching a filter, newest
// admission first, continuing after cursor. An empty sessionID pages across every
// session and empty statuses match every lifecycle position — the whole history,
// because a finished Run is still the answer to what a session did and cost.
//
// It reads the durable admission record rather than a live in-process registry:
// the registry only knows the segments THIS process is streaming, so it answers a
// different question, and answers it differently after a restart.
//
// The cursor is bound to the normalized filter, not just to the method: continuing
// a page under a different session or status set would seek into a collection the
// anchor was never a position in.
func (c *Coordinator) ListRunPage(ctx context.Context, sessionID string, statuses []execution.RunStatus, cursor string, limit int) (keyset.Page[transcript.Run], error) {
	statuses = normalizeStatuses(statuses)
	filters := []string{sessionID, statusFilter(statuses)}
	beforeCreatedAt, beforeID, err := timeAndIDAnchor(cursor, runPageMethod, filters)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	size, err := keyset.Limit(limit, runPageLimit)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	rows, err := c.runs.PageRuns(ctx, sessionID, statuses, beforeCreatedAt, beforeID, size+1)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	return keyset.PageOf(rows, size, runPageMethod, filters, func(run transcript.Run) []string {
		return []string{strconv.FormatInt(run.CreatedAt.UnixNano(), 10), run.ID}
	}), nil
}

// normalizeStatuses puts a status set in one canonical order and drops repeats, so
// two requests asking for the same set mint the same cursor. Sorting is by the
// domain's own declaration order — the enum IS the order, so there is nothing else
// to agree with.
func normalizeStatuses(statuses []execution.RunStatus) []execution.RunStatus {
	if len(statuses) == 0 {
		return nil
	}
	normalized := slices.Clone(statuses)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

// statusFilter is the normalized status set as one cursor filter value. The empty
// set is every status, which is a different collection from any explicit set — and
// it reads as such, since no status is spelled "".
func statusFilter(statuses []execution.RunStatus) string {
	names := make([]string, 0, len(statuses))
	for _, status := range statuses {
		names = append(names, status.String())
	}
	return strings.Join(names, ",")
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
		return []string{strconv.FormatInt(pending.CreatedAt.UnixNano(), 10), pending.RootRunID}
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
