// Package queries is the application-owned read surface over a session's durable
// execution record: the transcript (items + runs) and open HITL interrupts.
// These are projections read directly from persistence (§5.4) — no aggregate is
// loaded and no command store is fattened with reads. Delivery drives them for
// runs.list, items.list and interrupts.list.
package queries

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
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
	interruptPageMethod = "interrupts.list"
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
	PageRunTreeItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
}

// TodoReader is the coordinator's view of the task-list projection: the whole
// state, because what identifies one replacement from the next is its revision.
type TodoReader interface {
	State(ctx context.Context, sessionID string) (todo.State, error)
}

// SessionReader answers only whether a session exists. The item read needs that
// much and no more: a scope naming no session is a refusal, and an empty page would
// tell the client its session is empty instead.
type SessionReader interface {
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// InterruptReader is the coordinator's view of the open-interrupt registry. Both
// filters are optional and independent: empty means "every", and given together they
// both apply.
type InterruptReader interface {
	ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]interrupts.Pending, error)
}

// RunReader is the coordinator's view of the durable Run record: one Run by id,
// the ancestor closure of a named set, and a browsable page of Runs. The closure
// threads a page of items onto a connected tree without loading unrelated Runs
// from a long session.
type RunReader interface {
	Run(ctx context.Context, runID string) (transcript.Run, bool, error)
	RunsWithAncestors(ctx context.Context, runIDs []string) ([]transcript.Run, error)
	PageRuns(ctx context.Context, sessionID string, statuses []execution.RunStatus, includeDescendants bool, beforeCreatedAt int64, beforeRunID string, limit int) ([]transcript.Run, error)
}

// Coordinator serves the session read projections. Stateless beyond its store
// collaborators; safe to share.
type Coordinator struct {
	transcript TranscriptReader
	interrupts InterruptReader
	runs       RunReader
	sessions   SessionReader
	todos      TodoReader
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Transcript TranscriptReader
	Interrupts InterruptReader
	Runs       RunReader
	Sessions   SessionReader
	Todos      TodoReader
}

// New returns a query Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		transcript: deps.Transcript,
		interrupts: deps.Interrupts,
		runs:       deps.Runs,
		sessions:   deps.Sessions,
		todos:      deps.Todos,
	}
}

// ItemPage is one page of a session's history, with the run tree needed to thread
// the items on it.
type ItemPage struct {
	Items      []transcript.Item
	NextCursor string
	Runs       []transcript.Run
}

type itemScopeKind uint8

const (
	sessionItemScope itemScopeKind = iota + 1
	runItemScope
)

var errInvalidItemScope = errors.New("queries: item scope is invalid")

// ItemScope is the closed application choice of a whole session timeline, one
// Run's own items, or that Run's complete subtree. Its fields stay private so
// callers cannot construct a scope that names both a Session and a Run, or ask a
// Session for descendants it already contains.
type ItemScope struct {
	kind               itemScopeKind
	subjectID          string
	includeDescendants bool
}

// SessionItems scopes a page to a session's whole timeline.
func SessionItems(sessionID string) ItemScope {
	return ItemScope{kind: sessionItemScope, subjectID: sessionID}
}

// RunItems scopes a page to one Run's own items. The Run's session is resolved from
// the Run, so no caller has to supply both and risk supplying two different ones.
func RunItems(runID string) ItemScope {
	return ItemScope{kind: runItemScope, subjectID: runID}
}

// RunTreeItems scopes a page to one Run and all of its descendants. The subject
// may itself be a child; ancestors are not part of the item scope.
func RunTreeItems(runID string) ItemScope {
	return ItemScope{
		kind:               runItemScope,
		subjectID:          runID,
		includeDescendants: true,
	}
}

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
	filters, err := scope.cursorFilters(order)
	if err != nil {
		return ItemPage{}, err
	}
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
	runs, err := c.runs.RunsWithAncestors(ctx, referencedRuns(items))
	if err != nil {
		return ItemPage{}, err
	}
	return ItemPage{Items: items, NextCursor: page.NextCursor, Runs: runs}, nil
}

// TodoState returns a session's task-list projection. A session that exists but has
// never written a list is the zero state — revision 0, no items — because "nothing
// has been written" is an answer, not a gap. Only a session that does not exist is
// [session.ErrNotFound]: an empty list for an id nobody owns would read as a real,
// empty session.
func (c *Coordinator) TodoState(ctx context.Context, sessionID string) (todo.State, error) {
	found, err := c.sessions.Exists(ctx, sessionID)
	if err != nil {
		return todo.State{}, err
	}
	if !found {
		return todo.State{}, session.ErrNotFound
	}
	return c.todos.State(ctx, sessionID)
}

// requireScope refuses a scope whose subject does not exist. It runs after the
// cursor and limit are validated: a malformed request is malformed whether or not
// its subject exists, and answering "no such session" to a request that was never
// answerable would send the caller looking in the wrong place.
func (c *Coordinator) requireScope(ctx context.Context, scope ItemScope) error {
	switch scope.kind {
	case runItemScope:
		if _, found, err := c.runs.Run(ctx, scope.subjectID); err != nil || !found {
			return cmp.Or(err, transcript.ErrRunNotFound)
		}
		return nil
	case sessionItemScope:
		found, err := c.sessions.Exists(ctx, scope.subjectID)
		if err != nil {
			return err
		}
		if !found {
			return session.ErrNotFound
		}
		return nil
	default:
		return errInvalidItemScope
	}
}

func (c *Coordinator) readScope(ctx context.Context, scope ItemScope, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	switch scope.kind {
	case runItemScope:
		if scope.includeDescendants {
			return c.transcript.PageRunTreeItems(ctx, scope.subjectID, order, fromSequence, limit)
		}
		return c.transcript.PageRunItems(ctx, scope.subjectID, order, fromSequence, limit)
	case sessionItemScope:
		return c.transcript.PageSessionItems(ctx, scope.subjectID, order, fromSequence, limit)
	default:
		return nil, errInvalidItemScope
	}
}

func (scope ItemScope) cursorFilters(order transcript.SequenceOrder) ([]string, error) {
	switch scope.kind {
	case sessionItemScope:
		return []string{scope.subjectID, "", strconv.FormatBool(false), order.String()}, nil
	case runItemScope:
		return []string{"", scope.subjectID, strconv.FormatBool(scope.includeDescendants), order.String()}, nil
	default:
		return nil, errInvalidItemScope
	}
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

// RunPageFilter is every collection-defining input to [Coordinator.ListRunPage].
// Keeping it named makes includeDescendants part of the query rather than an
// easily misplaced positional boolean, and gives the cursor one explicit filter
// identity to bind.
type RunPageFilter struct {
	SessionID          string
	Statuses           []execution.RunStatus
	IncludeDescendants bool
}

// ListRunPage returns one page of Runs matching filter, newest admission first,
// continuing after cursor. An empty SessionID pages across every session, empty
// Statuses match every lifecycle position, and IncludeDescendants false restricts
// the page to roots. A finished Run remains part of history.
//
// It reads the durable admission record rather than a live in-process registry:
// the registry only knows the segments THIS process is streaming, so it answers a
// different question, and answers it differently after a restart.
//
// The cursor is bound to the normalized filter, not just to the method: continuing
// a page under a different session or status set would seek into a collection the
// anchor was never a position in.
func (c *Coordinator) ListRunPage(ctx context.Context, filter RunPageFilter, cursor string, limit int) (keyset.Page[transcript.Run], error) {
	filter.Statuses = normalizeStatuses(filter.Statuses)
	filters := []string{
		filter.SessionID,
		statusFilter(filter.Statuses),
		strconv.FormatBool(filter.IncludeDescendants),
	}
	beforeCreatedAt, beforeID, err := timeAndIDAnchor(cursor, runPageMethod, filters)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	size, err := keyset.Limit(limit, runPageLimit)
	if err != nil {
		return keyset.Page[transcript.Run]{}, err
	}
	rows, err := c.runs.PageRuns(
		ctx,
		filter.SessionID,
		filter.Statuses,
		filter.IncludeDescendants,
		beforeCreatedAt,
		beforeID,
		size+1,
	)
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

// ListPendingInterruptPage returns one page of the durable waiting sets, the
// longest wait first, continuing after cursor. An empty sessionID pages across
// every session and an empty rootRunID every waiting tree; given together they must
// both match.
//
// The page unit is a whole set, never an interrupt: [runs.Resume] validates and
// consumes a set in one transaction, so half a set is a resume nobody can attempt.
// Sets are one row each, which is what makes "never split" a property of the
// storage rather than a rule this read has to remember.
//
// caller is what the requesting client declared it can follow. A set whose Run
// publishes more than that is REFUSED — [execution.ErrProfileNotCovered] — rather
// than returned with the parts the caller understands: a client that answered a
// trimmed set would leave the rest of it open forever, and the run would stay
// waiting on interrupts the client believes it resolved.
//
// rootRunID must name a root. A child id is [transcript.ErrNotRoot], because the
// set it belongs to exists — under the root — and an empty page would say otherwise.
func (c *Coordinator) ListPendingInterruptPage(ctx context.Context, sessionID, rootRunID string, caller execution.RunProtocolProfile, cursor string, limit int) (keyset.Page[interrupts.Pending], error) {
	filters := []string{sessionID, rootRunID}
	afterCreatedAt, afterID, err := timeAndIDAnchor(cursor, interruptPageMethod, filters)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	size, err := keyset.Limit(limit, interruptPageLimit)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	if err := c.requireRoot(ctx, rootRunID); err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	rows, err := c.interrupts.ListPage(ctx, sessionID, rootRunID, afterCreatedAt, afterID, size+1)
	if err != nil {
		return keyset.Page[interrupts.Pending]{}, err
	}
	page := keyset.PageOf(rows, size, interruptPageMethod, filters, func(pending interrupts.Pending) []string {
		return []string{strconv.FormatInt(pending.CreatedAt.UnixNano(), 10), pending.RootRunID}
	})
	for _, pending := range page.Rows {
		if gap := pending.ProtocolProfile.Uncovered(caller); !gap.IsEmpty() {
			return keyset.Page[interrupts.Pending]{}, &execution.ProfileNotCovered{RunID: pending.RootRunID, Gap: gap}
		}
	}
	return page, nil
}

// requireRoot refuses a run filter that names a child. An empty filter names no run
// and is not checked; a filter naming nothing that exists is left to the page, which
// returns none — "no such run" and "that run has nothing waiting" are the same
// answer to the caller, while "you named a child" is not.
func (c *Coordinator) requireRoot(ctx context.Context, runID string) error {
	if runID == "" {
		return nil
	}
	run, found, err := c.runs.Run(ctx, runID)
	if err != nil || !found {
		return err
	}
	if run.Lineage().IsChild() {
		return fmt.Errorf("%w: run %q belongs to the tree rooted elsewhere", transcript.ErrNotRoot, runID)
	}
	return nil
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
