package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// defaultTranscriptSearchLimit caps a search_conversations that names no limit.
const defaultTranscriptSearchLimit = 10

type TranscriptStore struct{ db *sql.DB }

func NewTranscriptStore(db *sql.DB) *TranscriptStore { return &TranscriptStore{db: db} }

func (s *TranscriptStore) AppendItem(ctx context.Context, item transcript.Item) error {
	if item.SessionID == "" {
		return errors.New("sqlite: history item sessionId is required")
	}
	if item.ID == "" {
		return errors.New("sqlite: history item id is required")
	}
	var offloadID offload.ID
	if item.Tool != nil && item.Tool.Offload != nil {
		if err := item.Tool.Offload.Validate(); err != nil {
			return fmt.Errorf("sqlite: history item offload: %w", err)
		}
		if item.Tool.Result == nil {
			return errors.New("sqlite: offloaded history item result is absent")
		}
		if _, ok := item.Tool.Result.String(); !ok {
			return errors.New("sqlite: offloaded history item result must be a preview string")
		}
		offloadID = item.Tool.Offload.ID
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("sqlite: encode history item: %w", err)
	}
	searchText, searchable := transcript.SearchableText(item)
	// The history write and its full-text index maintenance are one atomic
	// write-set (RunInTx joins any outer cross-store transaction), so the search
	// index never drifts from the transcript it mirrors.
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		res, err := q.ExecContext(ctx,
			`INSERT INTO history_items(session_id, run_id, item_id, created_at, payload, offload_id)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(item_id) DO UPDATE SET
			   payload = excluded.payload,
			   offload_id = excluded.offload_id
			 WHERE history_items.session_id = excluded.session_id
			   AND history_items.run_id = excluded.run_id
			   AND (history_items.offload_id = '' OR history_items.offload_id = excluded.offload_id)`,
			item.SessionID, item.RunID, item.ID, item.CreatedAt.UnixNano(), string(payload), offloadID,
		)
		if err != nil {
			if offloadID != "" {
				var ownerItem string
				ownerErr := q.QueryRowContext(ctx,
					`SELECT item_id FROM history_items WHERE offload_id = ?`, offloadID,
				).Scan(&ownerItem)
				if ownerErr == nil && ownerItem != item.ID {
					return fmt.Errorf("%w: offload %q already belongs to item %q", transcript.ErrIdentityConflict, offloadID, ownerItem)
				}
				if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
					return fmt.Errorf("sqlite: inspect history item offload conflict: %w", errors.Join(err, ownerErr))
				}
			}
			return fmt.Errorf("sqlite: append history item: %w", err)
		}
		if changed, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("sqlite: inspect history item write: %w", err)
		} else if changed != 1 {
			return fmt.Errorf("%w: item %q already belongs to another session, run, or offload identity", transcript.ErrIdentityConflict, item.ID)
		}
		if searchable {
			return s.indexForSearch(ctx, item, searchText)
		}
		return nil
	})
}

// Item resolves one durable transcript Item by its globally unique identity.
// The returned value is the same fully hydrated projection as List/Page reads.
func (s *TranscriptStore) Item(ctx context.Context, itemID string) (transcript.Item, bool, error) {
	if strings.TrimSpace(itemID) == "" {
		return transcript.Item{}, false, errors.New("sqlite: history item id is required")
	}
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id, run_id, item_id, created_at, payload, offload_id,
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
func (s *TranscriptStore) ReplaceItem(
	ctx context.Context,
	expected transcript.Item,
	replacement transcript.Item,
) error {
	if expected.ID == "" || replacement.ID == "" {
		return errors.New("sqlite: replace history item requires both item ids")
	}
	if expected.ID != replacement.ID ||
		expected.SessionID != replacement.SessionID ||
		expected.RunID != replacement.RunID {
		return fmt.Errorf("%w: replacement changes item %q ownership", transcript.ErrIdentityConflict, expected.ID)
	}
	if (expected.Tool != nil && expected.Tool.Offload != nil) ||
		(replacement.Tool != nil && replacement.Tool.Offload != nil) {
		return errors.New("sqlite: replace history item does not support offloaded tool results")
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("sqlite: replace history item %q expected value: %w", expected.ID, err)
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: replace history item %q replacement: %w", replacement.ID, err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.Item(ctx, expected.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("sqlite: replace history item %q: not found", expected.ID)
		}
		if !reflect.DeepEqual(current, expected) {
			return fmt.Errorf(
				"%w: item %q changed after the application prepared its replacement",
				transcript.ErrIdentityConflict,
				expected.ID,
			)
		}
		return s.AppendItem(ctx, replacement)
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
		createdAt    int64
	)
	if err := row.Scan(
		&sessionID,
		&runID,
		&itemID,
		&createdAt,
		&payload,
		&rawOffloadID,
		&offloaded,
	); err != nil {
		return transcript.Item{}, err
	}
	var item transcript.Item
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decode history item %q: %w", itemID, err)
	}
	item.SessionID = sessionID
	item.RunID = runID
	item.ID = itemID
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	if rawOffloadID == "" {
		return item, nil
	}
	id, err := offload.ParseID(rawOffloadID)
	if err != nil {
		return transcript.Item{}, fmt.Errorf("sqlite: decode history item %q offload: %w", itemID, err)
	}
	if item.Tool == nil {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", itemID)
	}
	if item.Tool.Result == nil {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview", itemID)
	}
	if _, ok := item.Tool.Result.String(); !ok {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q has an offload identity but no preview string", itemID)
	}
	if !offloaded.Valid {
		return transcript.Item{}, fmt.Errorf("sqlite: history item %q references missing tool result %q", itemID, id)
	}
	item.Tool.Offload = &offload.Ref{ID: id}
	body := tool.StringResult(offloaded.String)
	item.Tool.Result = &body
	return item, nil
}

// indexForSearch write-through-indexes a conversation item for search_conversations,
// keyed by the item's history seq so the FTS rowid stays aligned as the item
// grows (a streamed agent message re-appends with the full text). FTS5 has no
// rowid upsert, so it is delete-then-insert. Must run inside AppendItem's
// transaction, after the history_items row exists.
func (s *TranscriptStore) indexForSearch(ctx context.Context, item transcript.Item, text string) error {
	q := conn(ctx, s.db)
	var seq int64
	if err := q.QueryRowContext(ctx, `SELECT seq FROM history_items WHERE item_id = ?`, item.ID).Scan(&seq); err != nil {
		return fmt.Errorf("sqlite: locate history item for search index: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM transcript_search WHERE rowid = ?`, seq); err != nil {
		return fmt.Errorf("sqlite: clear search index row: %w", err)
	}
	if _, err := q.ExecContext(ctx,
		`INSERT INTO transcript_search(rowid, text, session_id, run_id, item_id, kind, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		seq, text, item.SessionID, item.RunID, item.ID, int(item.Kind), item.CreatedAt.UnixNano(),
	); err != nil {
		return fmt.Errorf("sqlite: index history item for search: %w", err)
	}
	return nil
}

// DeleteRun removes one run's items from a session's history. The Run's own row
// belongs to the run store; this store owns the item log.
func (s *TranscriptStore) DeleteRun(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete history run requires sessionId + runId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
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

func (s *TranscriptStore) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("sqlite: delete history session requires sessionId")
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
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
func (s *TranscriptStore) List(ctx context.Context, sessionID string) ([]transcript.Item, error) {
	sequenced, err := s.PageSessionItems(ctx, sessionID, transcript.OldestFirst, 0, 0)
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
func (s *TranscriptStore) PageSessionItems(ctx context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return s.pageItems(ctx, `h.session_id = ?`, sessionID, order, fromSequence, limit)
}

// PageRunItems is the same page over one run's own items. The run id needs no
// session beside it: it identifies exactly one run, and a run belongs to one
// session.
func (s *TranscriptStore) PageRunItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return s.pageItems(ctx, `h.run_id = ?`, runID, order, fromSequence, limit)
}

// PageRunTreeItems returns one Run's items plus every descendant's, using the
// durable parent edge as the subtree authority. The transcript never infers
// lineage from event order or spawning-item contents.
func (s *TranscriptStore) PageRunTreeItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	return s.pageItems(ctx, `h.run_id IN (
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

func (s *TranscriptStore) pageItems(ctx context.Context, scope, subject string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	query := `SELECT h.seq, h.session_id, h.run_id, h.item_id, h.created_at, h.payload, h.offload_id, b.body
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
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list history items: %w", err)
	}
	defer rows.Close()

	var out []transcript.SequencedItem
	for rows.Next() {
		var session, runID, itemID, payload, rawOffloadID string
		var offloadedBody sql.NullString
		var sequence, createdAt int64
		if err := rows.Scan(&sequence, &session, &runID, &itemID, &createdAt, &payload, &rawOffloadID, &offloadedBody); err != nil {
			return nil, fmt.Errorf("sqlite: scan history item: %w", err)
		}
		var item transcript.Item
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			return nil, fmt.Errorf("sqlite: decode history item %q: %w", itemID, err)
		}
		item.SessionID = session
		item.RunID = runID
		item.ID = itemID
		item.CreatedAt = time.Unix(0, createdAt).UTC()
		if rawOffloadID != "" {
			id, err := offload.ParseID(rawOffloadID)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode history item %q offload: %w", itemID, err)
			}
			if item.Tool == nil {
				return nil, fmt.Errorf("sqlite: history item %q has an offload identity but no tool invocation", itemID)
			}
			if item.Tool.Result == nil {
				return nil, fmt.Errorf("sqlite: history item %q has an offload identity but no preview", itemID)
			}
			if _, ok := item.Tool.Result.String(); !ok {
				return nil, fmt.Errorf("sqlite: history item %q has an offload identity but no preview string", itemID)
			}
			if !offloadedBody.Valid {
				return nil, fmt.Errorf("sqlite: history item %q references missing tool result %q", itemID, id)
			}
			item.Tool.Offload = &offload.Ref{ID: id}
			body := tool.StringResult(offloadedBody.String)
			item.Tool.Result = &body
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
func (s *TranscriptStore) SearchTranscript(ctx context.Context, query string, limit int) ([]transcript.SearchHit, error) {
	match := ftsMatchQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultTranscriptSearchLimit
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
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
		var kind int
		var createdAt int64
		if err := rows.Scan(&sessionID, &runID, &itemID, &kind, &createdAt, &snippet); err != nil {
			return nil, fmt.Errorf("sqlite: scan transcript search hit: %w", err)
		}
		out = append(out, transcript.SearchHit{
			SessionID: sessionID,
			RunID:     runID,
			ItemID:    itemID,
			Kind:      transcript.ItemKind(kind),
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
