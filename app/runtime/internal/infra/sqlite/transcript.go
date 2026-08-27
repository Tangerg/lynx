package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// defaultTranscriptSearchLimit caps a transcript search that names no limit.
const defaultTranscriptSearchLimit = 10

type TranscriptStore struct{ db *sql.DB }

func NewTranscriptStore(db *sql.DB) *TranscriptStore { return &TranscriptStore{db: db} }

func (t *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	if item.SessionID() == "" {
		return errors.New("sqlite: history item sessionId is required")
	}
	if item.ID() == "" {
		return errors.New("sqlite: history item id is required")
	}
	if err := item.Validate(); err != nil {
		return fmt.Errorf("sqlite: history item %q: %w", item.ID(), err)
	}
	offloadID, err := transcriptOffloadID(item)
	if err != nil {
		return err
	}
	payload, err := encodeTranscriptItem(item)
	if err != nil {
		return fmt.Errorf("sqlite: encode history item: %w", err)
	}
	searchText, searchable := transcript.SearchableText(item)
	// The history write and its full-text index maintenance are one atomic
	// write-set (RunInTx joins any outer cross-store transaction), so the search
	// index never drifts from the transcript it mirrors.
	return RunInTx(ctx, t.db, func(ctx context.Context) error {
		if err := t.appendItemRecord(ctx, item, payload, offloadID); err != nil {
			return err
		}
		if searchable {
			return t.indexForSearch(ctx, item, searchText)
		}
		return nil
	})
}

func transcriptOffloadID(item transcript.Item) (toolresult.ID, error) {
	invocation, present := item.ToolInvocation()
	if !present || invocation.Offload == nil {
		return "", nil
	}
	if err := invocation.Offload.Validate(); err != nil {
		return "", fmt.Errorf("sqlite: history item offload: %w", err)
	}
	if invocation.Result == nil {
		return "", errors.New("sqlite: offloaded history item result is absent")
	}
	if _, ok := invocation.Result.String(); !ok {
		return "", errors.New("sqlite: offloaded history item result must be a preview string")
	}
	return invocation.Offload.ID, nil
}

func (t *TranscriptStore) appendItemRecord(
	ctx context.Context,
	item transcript.Item,
	payload []byte,
	offloadID toolresult.ID,
) error {
	q := conn(ctx, t.db)
	result, err := q.ExecContext(ctx,
		`INSERT INTO history_items(session_id, run_id, item_id, occurred_at, payload, offload_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET
		   payload = excluded.payload,
		   offload_id = excluded.offload_id
		 WHERE history_items.session_id = excluded.session_id
		   AND history_items.run_id = excluded.run_id
		   AND history_items.occurred_at = excluded.occurred_at
		   AND (history_items.offload_id = '' OR history_items.offload_id = excluded.offload_id)`,
		item.SessionID(), item.RunID(), item.ID(), item.OccurredAt().UnixNano(), string(payload), offloadID,
	)
	if err != nil {
		return t.explainItemAppendError(ctx, item.ID(), offloadID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect history item write: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: item %q already belongs to another session, run, occurrence, or offload identity", transcript.ErrIdentityConflict, item.ID())
	}
	return nil
}

func (t *TranscriptStore) explainItemAppendError(
	ctx context.Context,
	itemID string,
	offloadID toolresult.ID,
	appendErr error,
) error {
	if offloadID == "" {
		return fmt.Errorf("sqlite: append history item: %w", appendErr)
	}
	var ownerItemID string
	ownerErr := conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT item_id FROM history_items WHERE offload_id = ?`, offloadID,
	).Scan(&ownerItemID)
	if ownerErr == nil && ownerItemID != itemID {
		return fmt.Errorf("%w: offload %q already belongs to item %q", transcript.ErrIdentityConflict, offloadID, ownerItemID)
	}
	if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
		return fmt.Errorf("sqlite: inspect history item offload conflict: %w", errors.Join(appendErr, ownerErr))
	}
	return fmt.Errorf("sqlite: append history item: %w", appendErr)
}

// Item resolves one durable transcript Item by its globally unique identity.
// The returned value is the same fully hydrated projection as List/Page reads.
func (t *TranscriptStore) Item(ctx context.Context, itemID string) (transcript.Item, bool, error) {
	if strings.TrimSpace(itemID) == "" {
		return transcript.Item{}, false, errors.New("sqlite: history item id is required")
	}
	row := conn(ctx, t.db).QueryRowContext(ctx,
		`SELECT session_id, run_id, item_id, occurred_at, payload, offload_id,
		        (SELECT body FROM tool_result_blobs WHERE id = history_items.offload_id
		          AND session_id = history_items.session_id AND item_id = history_items.item_id)
		   FROM history_items
		  WHERE item_id = ?`,
		itemID,
	)
	item, err := scanTranscriptItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return transcript.Item{}, false, nil
	}
	if err != nil {
		return transcript.Item{}, false, err
	}
	return item, true, nil
}

// ReplaceItem atomically replaces expected with replacement only while the
// durable Item still equals the immutable value the application planned
// against. This is the transcript-side compare-and-swap used by tree
// transformations: a racing continuation can never have its newer completion
// overwritten by a cancellation built from an older parked projection.
func (t *TranscriptStore) ReplaceItem(
	ctx context.Context,
	expected transcript.Item,
	replacement transcript.Item,
) error {
	if expected.ID() == "" || replacement.ID() == "" {
		return errors.New("sqlite: replace history item requires both item ids")
	}
	if expected.ID() != replacement.ID() ||
		expected.SessionID() != replacement.SessionID() ||
		expected.RunID() != replacement.RunID() {
		return fmt.Errorf("%w: replacement changes item %q ownership", transcript.ErrIdentityConflict, expected.ID())
	}
	expectedTool, expectedHasTool := expected.ToolInvocation()
	replacementTool, replacementHasTool := replacement.ToolInvocation()
	if (expectedHasTool && expectedTool.Offload != nil) ||
		(replacementHasTool && replacementTool.Offload != nil) {
		return errors.New("sqlite: replace history item does not support offloaded tool results")
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("sqlite: replace history item %q expected value: %w", expected.ID(), err)
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: replace history item %q replacement: %w", replacement.ID(), err)
	}
	return RunInTx(ctx, t.db, func(ctx context.Context) error {
		current, found, err := t.Item(ctx, expected.ID())
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("sqlite: replace history item %q: not found", expected.ID())
		}
		if !reflect.DeepEqual(current.Snapshot(), expected.Snapshot()) {
			return fmt.Errorf(
				"%w: item %q changed after the application prepared its replacement",
				transcript.ErrIdentityConflict,
				expected.ID(),
			)
		}
		return t.AppendItem(ctx, replacement)
	})
}

func scanTranscriptItem(row scanRow) (transcript.Item, error) {
	var (
		sessionID    string
		runID        string
		itemID       string
		payload      string
		rawOffloadID string
		offloaded    sql.NullString
		occurredAt   int64
	)
	if err := row.Scan(
		&sessionID,
		&runID,
		&itemID,
		&occurredAt,
		&payload,
		&rawOffloadID,
		&offloaded,
	); err != nil {
		return transcript.Item{}, err
	}
	return materializeTranscriptItem(sessionID, runID, itemID, occurredAt, payload, rawOffloadID, offloaded)
}

func materializeTranscriptItem(
	sessionID, runID, itemID string,
	occurredAt int64,
	payload, rawOffloadID string,
	offloaded sql.NullString,
) (transcript.Item, error) {
	snapshot, err := decodeTranscriptItem([]byte(payload))
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decode history item %q: %w", itemID, err)
	}
	snapshot.Identity = transcript.ItemIdentity{
		SessionID: sessionID, RunID: runID, ItemID: itemID,
		OccurredAt: time.Unix(0, occurredAt).UTC(),
	}
	if rawOffloadID == "" {
		item, restoreItemErr := transcript.RestoreItem(snapshot)
		if restoreItemErr != nil {
			return transcript.Item{}, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, restoreItemErr)
		}
		return item, nil
	}
	id, err := toolresult.ParseID(rawOffloadID)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decode history item %q offload: %w", itemID, err)
	}
	if snapshot.Tool == nil {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", itemID)
	}
	if snapshot.Tool.Result == nil {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview", itemID)
	}
	if _, ok := snapshot.Tool.Result.String(); !ok {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview string", itemID)
	}
	if !offloaded.Valid {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q references missing tool result %q", itemID, id)
	}
	snapshot.Tool.Offload = &toolresult.Ref{ID: id}
	body := tool.StringResult(offloaded.String)
	snapshot.Tool.Result = &body
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decoded history item %q: %w", itemID, err)
	}
	return item, nil
}

// indexForSearch write-through-indexes a conversation item for transcript search,
// keyed by the item's history seq so the FTS rowid stays aligned as the item
// grows (a streamed agent message re-appends with the full text). FTS5 has no
// rowid upsert, so it is delete-then-insert. Must run inside AppendItem's
// transaction, after the history_items row exists.
func (t *TranscriptStore) indexForSearch(ctx context.Context, item transcript.Item, text string) error {
	q := conn(ctx, t.db)
	var seq int64
	if err := q.QueryRowContext(ctx, `SELECT seq FROM history_items WHERE item_id = ?`, item.ID()).Scan(&seq); err != nil {
		return fmt.Errorf("sqlite: locate history item for search index: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM transcript_search WHERE rowid = ?`, seq); err != nil {
		return fmt.Errorf("sqlite: clear search index row: %w", err)
	}
	if _, err := q.ExecContext(ctx,
		`INSERT INTO transcript_search(rowid, text, session_id, run_id, item_id, kind, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		seq, text, item.SessionID(), item.RunID(), item.ID(), item.Kind(), item.OccurredAt().UnixNano(),
	); err != nil {
		return fmt.Errorf("sqlite: index history item for search: %w", err)
	}
	return nil
}

// DeleteRun removes one run's items from a session's history. The Run's own row
// belongs to the run store; this store owns the item log.
func (t *TranscriptStore) DeleteRun(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete history run requires sessionId + runId")
	}
	return RunInTx(ctx, t.db, func(ctx context.Context) error {
		q := conn(ctx, t.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM tool_result_blobs
			 WHERE item_id IN (
			   SELECT item_id FROM history_items WHERE session_id = ? AND run_id = ?
			 )`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run tool results: %w", err)
		}
		// Clear the search index rows (keyed by history seq) before the items they
		// mirror are deleted, so no stale hits survive.
		if _, err := q.ExecContext(ctx,
			`DELETE FROM transcript_search
			 WHERE rowid IN (
			   SELECT seq FROM history_items WHERE session_id = ? AND run_id = ?
			 )`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run search index: %w", err)
		}
		if _, err := q.ExecContext(ctx,
			`DELETE FROM history_items WHERE session_id = ? AND run_id = ?`, sessionID, runID,
		); err != nil {
			return fmt.Errorf("sqlite: delete run items: %w", err)
		}
		return nil
	})
}

func (t *TranscriptStore) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("sqlite: delete history session requires sessionId")
	}
	return RunInTx(ctx, t.db, func(ctx context.Context) error {
		q := conn(ctx, t.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM transcript_search
			 WHERE rowid IN (SELECT seq FROM history_items WHERE session_id = ?)`, sessionID,
		); err != nil {
			return fmt.Errorf("sqlite: delete session search index: %w", err)
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM history_items WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: delete session items: %w", err)
		}
		return nil
	})
}

// List returns a session's whole item history in durable append order.
func (t *TranscriptStore) List(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	sequenced, err := t.PageSessionItems(ctx, sessionID, transcript.OldestFirst, 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]transcript.Item, 0, len(sequenced))
	for _, entry := range sequenced {
		out = append(out, entry.Item)
	}
	return out, nil
}

// PageSessionItems returns one page of a session's history along the durable
// sequence, in the direction order names. fromSequence is the position a previous
// page ended at; zero is no anchor, which is exact because the sequence is
// 1-based — so the same zero means "from the beginning" reading forwards and "from
// the end" reading backwards. A zero limit reads to the end.
//
// The bound is applied by the query, not by the caller: seeking past an anchor and
// stopping at a limit is what keeps a long session's history out of memory when
// only a page of it was asked for.
func (t *TranscriptStore) PageSessionItems(ctx context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return t.pageItems(ctx, `h.session_id = ?`, sessionID, order, fromSequence, limit)
}

// PageRunItems is the same page over one run's own items. The run id needs no
// session beside it: it identifies exactly one run, and a run belongs to one
// session.
func (t *TranscriptStore) PageRunItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return t.pageItems(ctx, `h.run_id = ?`, runID, order, fromSequence, limit)
}

// PageRunTreeItems returns one Run's items plus every descendant's, using the
// durable parent edge as the subtree authority. The transcript never infers
// lineage from event order or spawning-item contents.
func (t *TranscriptStore) PageRunTreeItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return t.pageItems(ctx, `h.run_id IN (
		WITH RECURSIVE subtree(run_id) AS (
			SELECT run_id
			  FROM runs
			 WHERE run_id = ?
			UNION
			SELECT child.run_id
			  FROM runs AS child
			  JOIN subtree AS parent ON child.parent_run_id = parent.run_id
		)
		SELECT run_id FROM subtree
	)`, runID, order, fromSequence, limit)
}

func (t *TranscriptStore) pageItems(ctx context.Context, scope, subject string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	if err := order.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: page history items: %w", err)
	}
	if fromSequence < 0 {
		return nil, errors.New("sqlite: history page sequence must not be negative")
	}
	if limit < 0 {
		return nil, errors.New("sqlite: history page limit must not be negative")
	}
	query := `SELECT h.seq, h.session_id, h.run_id, h.item_id, h.occurred_at, h.payload, h.offload_id, b.body
		 FROM history_items AS h
		 LEFT JOIN tool_result_blobs AS b
		   ON b.id = h.offload_id AND b.session_id = h.session_id AND b.item_id = h.item_id
		 WHERE ` + scope
	args := []any{subject}
	bound, direction := `h.seq > ?`, ``
	if order == transcript.NewestFirst {
		bound, direction = `h.seq < ?`, ` DESC`
	}
	if fromSequence > 0 {
		query += ` AND ` + bound
		args = append(args, fromSequence)
	}
	query += ` ORDER BY h.seq` + direction
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := conn(ctx, t.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer rows.Close()

	var out []transcript.SequencedItem
	for rows.Next() {
		var session, runID, itemID, payload, rawOffloadID string
		var offloadedBody sql.NullString
		var sequence, occurredAt int64
		if err := rows.Scan(&sequence, &session, &runID, &itemID, &occurredAt, &payload, &rawOffloadID, &offloadedBody); err != nil {
			return nil, fmt.Errorf("sqlite: scan history item: %w", err)
		}
		item, err := materializeTranscriptItem(session, runID, itemID, occurredAt, payload, rawOffloadID, offloadedBody)
		if err != nil {
			return nil, err
		}
		out = append(out, transcript.SequencedItem{Sequence: sequence, Item: item})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	return out, nil
}

// SearchTranscript runs a full-text search over past conversation transcripts
// (user + agent messages across every session), most relevant first. query is
// natural-language keywords; a non-positive limit falls back to a default. An
// empty query returns no hits.
func (t *TranscriptStore) SearchTranscript(ctx context.Context, query string, limit int) ([]transcript.SearchHit, error) {
	match := ftsMatchQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultTranscriptSearchLimit
	}
	rows, err := conn(ctx, t.db).QueryContext(ctx,
		`SELECT session_id, run_id, item_id, kind, created_at,
		        snippet(transcript_search, 0, '[', ']', '…', 12)
		 FROM transcript_search
		 WHERE transcript_search MATCH ?
		 ORDER BY rank
		 LIMIT ?`, match, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: search transcripts: %w", err)
	}
	defer rows.Close()

	var out []transcript.SearchHit
	for rows.Next() {
		var sessionID, runID, itemID, snippet string
		var kind transcript.ItemKind
		var createdAt int64
		if err := rows.Scan(&sessionID, &runID, &itemID, &kind, &createdAt, &snippet); err != nil {
			return nil, fmt.Errorf("sqlite: scan transcript search hit: %w", err)
		}
		if !kind.Valid() {
			return nil, fmt.Errorf("sqlite: scan transcript search hit: unknown item kind %q", kind)
		}
		out = append(out, transcript.SearchHit{
			SessionID: sessionID,
			RunID:     runID,
			ItemID:    itemID,
			Kind:      kind,
			CreatedAt: time.Unix(0, createdAt).UTC(),
			Snippet:   snippet,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: search transcripts: %w", err)
	}
	return out, nil
}

// ftsMatchQuery turns natural-language input into a safe FTS5 MATCH expression:
// each whitespace-separated term becomes a quoted literal (so FTS5 operators or
// syntax in user text can't be interpreted or throw), joined by implicit AND —
// hits must contain every term. Empty input yields "".
func ftsMatchQuery(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, len(fields))
	for i, field := range fields {
		quoted[i] = `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " ")
}
