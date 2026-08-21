package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// Coarse admission states stored in runs.state. The partial unique index
// idx_runs_session_active keys on non-terminal root rows, so a Session holds at
// most one non-terminal Run tree while any number of its descendant rows may be
// active. The fine [run.Outcome] is stored separately in runs.outcome.
const (
	runStateRunning  = "running"
	runStateWaiting  = "waiting"
	runStateTerminal = "terminal"
)

// RunStore is the SQLite-backed Run table: one row per root or child Run,
// holding its durable projection. Its immutable lineage columns identify the
// tree without reconstructing it from transcript Items. A partial unique index
// guarantees at most one non-terminal root per Session across restarts; child
// rows share that root's admission.
//
// One table, one owner: the accrued facts are written only by the lifecycle
// transition that makes them true, so "where is this Run" and "how did it end"
// cannot disagree. A Run's open interrupts are the one part kept elsewhere: the
// interrupts table owns them and reads compose them.
type RunStore struct {
	db *sql.DB
}

// NewRunStore binds the Run table to db. db must have been opened via [Open] so
// the current schema was installed.
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// Admit records draft as the session's active (running) Run. It returns
// [rundomain.ErrSessionBusy] when the partial unique index rejects the INSERT —
// the session already has a non-terminal Run — and
// [rundomain.ErrIdentityConflict] when the Run ID is already taken, since the
// caller may supply one.
func (s *RunStore) Admit(ctx context.Context, draft rundomain.Draft) error {
	admitted, err := rundomain.Admit(draft)
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	lineage := admitted.Lineage()
	capabilities, err := encodeRunCapabilities(admitted.Capabilities())
	if err != nil {
		return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
	}
	now := admitted.CreatedAt().UnixNano()
	// This is the capability set's only writer, here and in Restore. Suspend,
	// resume, and finish deliberately do not name the column: the value cannot change
	// after admission, and the way to guarantee that is to have nothing able to
	// change it.
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		if lineage.IsChild() {
			if err := s.validateChildPlacement(
				ctx,
				"admit",
				draft.RunID,
				draft.SessionID,
				draft.ParentRunID,
				draft.RootRunID,
				true,
			); err != nil {
				return err
			}
		}
		_, err := conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO runs(
			   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
			   state, active_segment_id, provider, model, goal_incarnation_id, max_total_tokens, max_steps, max_budget_usd,
			   capabilities, message_mark, started_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			admitted.ID(), admitted.SessionID(),
			lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
			runStateRunning, admitted.ActiveSegmentID(),
			admitted.ModelSelection().Provider(), admitted.ModelSelection().Model(),
			admitted.GoalIncarnationID(),
			admitted.Limits().MaxTotalTokens, admitted.Limits().MaxSteps, admitted.Limits().MaxBudgetUSD, capabilities,
			rundomain.UnknownMessageMark, now, now)
		// Two constraints can reject this INSERT and they mean opposite things: the
		// primary key says the id is spoken for, the partial index says the Session
		// already owns another root tree.
		switch {
		case isPrimaryKeyViolation(err):
			return fmt.Errorf("%w: run %q already exists", rundomain.ErrIdentityConflict, draft.RunID)
		case isUniqueViolation(err):
			return rundomain.ErrSessionBusy
		case err != nil:
			return fmt.Errorf("sqlite: admit run %q: %w", draft.RunID, err)
		}
		return nil
	})
}

// validateChildPlacement proves immutable Run-to-Run topology before inserting
// a child. The spawning Item is validated by the application write-set that
// owns Item creation and child admission/restore together.
func (s *RunStore) validateChildPlacement(
	ctx context.Context,
	operation string,
	runID string,
	sessionID string,
	parentRunID string,
	rootRunID string,
	requireOpen bool,
) error {
	var (
		parentSession string
		parentRoot    string
		parentState   string
		rootSession   string
		rootParent    string
		rootState     string
	)
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT parent.session_id, parent.root_run_id, parent.state,
		        root.session_id, root.parent_run_id, root.state
		   FROM runs AS parent
		   JOIN runs AS root ON root.run_id = ?
		  WHERE parent.run_id = ?`,
		rootRunID,
		parentRunID,
	).Scan(
		&parentSession,
		&parentRoot,
		&parentState,
		&rootSession,
		&rootParent,
		&rootState,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q or root %q does not exist",
			operation,
			runID,
			parentRunID,
			rootRunID,
		)
	}
	if err != nil {
		return fmt.Errorf("sqlite: %s child run %q: validate tree: %w", operation, runID, err)
	}
	parentTreeRoot := parentRoot
	if parentTreeRoot == "" {
		parentTreeRoot = parentRunID
	}
	switch {
	case parentSession != sessionID:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q belongs to session %q, want %q",
			operation,
			runID,
			parentRunID,
			parentSession,
			sessionID,
		)
	case rootSession != sessionID:
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q belongs to session %q, want %q",
			operation,
			runID,
			rootRunID,
			rootSession,
			sessionID,
		)
	case rootParent != "":
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q is itself a child",
			operation,
			runID,
			rootRunID,
		)
	case parentTreeRoot != rootRunID:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q belongs to root %q, want %q",
			operation,
			runID,
			parentRunID,
			parentTreeRoot,
			rootRunID,
		)
	case requireOpen && parentState == runStateTerminal:
		return fmt.Errorf(
			"sqlite: %s child run %q: parent %q is terminal",
			operation,
			runID,
			parentRunID,
		)
	case requireOpen && rootState == runStateTerminal:
		return fmt.Errorf(
			"sqlite: %s child run %q: root %q is terminal",
			operation,
			runID,
			rootRunID,
		)
	}
	return nil
}

// Suspend persists the exact aggregate transition from Running to Waiting,
// recording what the Run had consumed up to the park. A missing row,
// repeated transition, mismatched identity, or any other source state is an
// ownership error and never succeeds silently.
func (s *RunStore) Suspend(ctx context.Context, value rundomain.Run) error {
	return s.suspend(ctx, value, runCommitIdentity{})
}

// SuspendBarrier parks one exact active Segment and stamps the root-owned tree
// barrier identity in the same transition. Child Runs use Suspend without a
// marker; the root marker proves the complete multi-Run transaction.
func (s *RunStore) SuspendBarrier(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
	commitID string,
) error {
	if err := validateRunCommitIdentity(value.SessionID(), value.ID(), segmentID, commitID); err != nil {
		return err
	}
	return s.suspend(ctx, value, runCommitIdentity{segmentID: segmentID, commitID: commitID})
}

func (s *RunStore) suspend(
	ctx context.Context,
	value rundomain.Run,
	commit runCommitIdentity,
) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
	}
	if value.State() != rundomain.Waiting {
		return fmt.Errorf("sqlite: suspend run %q: state is %s, want waiting", value.ID(), value.State())
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: suspend run %q: %w", value.ID(), err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.runForTransition(ctx, value.ID())
		if err != nil {
			return err
		}
		if !found || current.SessionID() != value.SessionID() {
			return errors.New("sqlite: suspend run: active run not found")
		}
		if commit.commitID != "" && current.ActiveSegmentID() != commit.segmentID {
			return fmt.Errorf(
				"sqlite: suspend run: active Segment is %q, want %q",
				current.ActiveSegmentID(), commit.segmentID,
			)
		}
		next, err := current.AdvanceProgress(
			value.Metrics(), value.ContextTokens(), value.UpdatedAt(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: advance aggregate metrics: %w", err)
		}
		next, err = next.Suspend(value.UpdatedAt())
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: %w", err)
		}
		if !next.Equal(value) {
			return errors.New("sqlite: suspend run: proposed Run differs from the aggregate transition")
		}
		// The segment identity is cleared in the same statement that parks the Run:
		// a Run waiting on a person has no segment to attach to.
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
			        steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next.State()), commit.segmentID, commit.commitID,
			metrics.steps, metrics.durationNs, metrics.usage, next.ContextTokens(), runUpdatedAt(value),
			value.SessionID(), value.ID(), coarseState(current.State()))
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: suspend run: read affected rows: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: suspend run: state changed concurrently (was %s)", current.State())
		}
		return nil
	})
}

// Resume continues the exact parked Run (Waiting → Running). Unlike cleanup
// transitions it is strict: a missing/mismatched/already-running row means the
// continuation opening does not own the durable Run and must roll back.
func (s *RunStore) Resume(
	ctx context.Context,
	sessionID string,
	draft rundomain.ResumeDraft,
	resumedAt time.Time,
) error {
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.runForTransition(ctx, draft.RunID)
		if err != nil {
			return err
		}
		if !found || current.SessionID() != sessionID {
			return errors.New("sqlite: resume run: active run not found")
		}
		next, err := current.Resume(draft.SegmentID, resumedAt)
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		// The accrual is untouched: a continuation inherits what the park committed,
		// and the segment now opening has consumed nothing yet. What does move is the
		// segment identity, which the park cleared and this one replaces.
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET state = ?, active_segment_id = ?, commit_segment_id = '', commit_id = '', updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`,
			coarseState(next.State()), next.ActiveSegmentID(), next.UpdatedAt().UnixNano(),
			sessionID, draft.RunID, coarseState(current.State()))
		if err != nil {
			return fmt.Errorf("sqlite: resume run: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: resume run: read affected rows: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: resume run: state changed concurrently (was %s)", current.State())
		}
		return nil
	})
}

// RequireActiveSegment proves that an event transaction still belongs to the
// exact running Segment that produced it. Callers execute this read through the
// transaction-bound connection before any projection write; a replacement,
// park, or terminal transition therefore rejects the complete stale write-set.
func (s *RunStore) RequireActiveSegment(ctx context.Context, sessionID, runID, segmentID string) error {
	if sessionID == "" || runID == "" || segmentID == "" {
		return errors.New("sqlite: require active Run Segment needs session, Run, and Segment identity")
	}
	var state, activeSegmentID string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT state, active_segment_id
		   FROM runs
		  WHERE session_id = ? AND run_id = ?`,
		sessionID,
		runID,
	).Scan(&state, &activeSegmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("sqlite: Run %q was not found in session %q", runID, sessionID)
	}
	if err != nil {
		return fmt.Errorf("sqlite: read active Segment for Run %q: %w", runID, err)
	}
	if state != runStateRunning || activeSegmentID != segmentID {
		return fmt.Errorf(
			"sqlite: Run %q is %s in Segment %q, want running Segment %q",
			runID,
			state,
			activeSegmentID,
			segmentID,
		)
	}
	return nil
}

// UpdateProgress records cumulative accounting and the latest prompt footprint
// observed at one model-call boundary while fencing both facts to the exact
// active segment. It never moves lifecycle state and rejects stale or regressing
// cumulative accounting.
func (s *RunStore) UpdateProgress(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	metrics rundomain.Metrics,
	contextTokens int64,
	updatedAt time.Time,
) error {
	if sessionID == "" || runID == "" || segmentID == "" {
		return errors.New("sqlite: update Run progress requires session, Run, and segment identity")
	}
	if updatedAt.IsZero() {
		return errors.New("sqlite: update Run progress requires an update time")
	}
	if err := metrics.Validate(); err != nil {
		return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
	}
	if contextTokens < 0 {
		return fmt.Errorf("sqlite: update Run progress for %q: context tokens must not be negative", runID)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.Run(ctx, runID)
		if err != nil {
			return err
		}
		if !found || current.SessionID() != sessionID {
			return fmt.Errorf("sqlite: update Run progress: running Run %q was not found in session %q", runID, sessionID)
		}
		if current.State() != rundomain.Running || current.ActiveSegmentID() != segmentID {
			return fmt.Errorf(
				"sqlite: update Run progress: Run %q is %s in segment %q, want running segment %q",
				runID,
				current.State(),
				current.ActiveSegmentID(),
				segmentID,
			)
		}
		next, err := current.AdvanceProgress(metrics, contextTokens, updatedAt)
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		encoded, err := runMetricsRow(next.Metrics())
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		result, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE runs SET steps = ?, active_duration_ns = ?, usage = ?, context_tokens = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
			encoded.steps,
			encoded.durationNs,
			encoded.usage,
			next.ContextTokens(),
			updatedAt.UTC().UnixNano(),
			sessionID,
			runID,
			runStateRunning,
			segmentID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: update Run progress for %q: %w", runID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Run progress update for %q: %w", runID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: update Run progress for %q lost its active-segment fence", runID)
		}
		return nil
	})
}

// Terminalize ends the exact non-terminal Run that run identifies, recording the
// outcome the executor reached and the result that explains it.
func (s *RunStore) Terminalize(ctx context.Context, value rundomain.Run) error {
	return s.terminalize(ctx, value, runCommitIdentity{})
}

// RecordRunCommit stamps one exact active Segment's latest immutable
// Application write-set identity into the Run row. Callers invoke it only at
// the end of the command transaction, after every projection has succeeded.
func (s *RunStore) RecordRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID string,
) error {
	if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = ?, commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
		segmentID,
		commitID,
		sessionID,
		runID,
		runStateRunning,
		segmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: Run commit %q lost its active-segment fence", commitID)
	}
	return nil
}

// RecordWaitingRunCommit stamps a command that transforms an already-waiting
// tree without opening a new Segment. The empty Segment is deliberate: the
// unique command identity and Waiting root own this boundary.
func (s *RunStore) RecordWaitingRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	commitID string,
) error {
	if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = '', commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ''`,
		commitID,
		sessionID,
		runID,
		runStateWaiting,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record waiting Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect waiting Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: waiting Run commit %q lost its state fence", commitID)
	}
	return nil
}

// TerminalizeEvent ends one exact active Segment and stamps the immutable
// Application EventCommit write-set identity into the Run row. The stamp shares
// the caller's transaction with every projection in that EventCommit.
func (s *RunStore) TerminalizeEvent(
	ctx context.Context,
	value rundomain.Run,
	segmentID string,
	commitID string,
) error {
	if err := validateRunCommitIdentity(value.SessionID(), value.ID(), segmentID, commitID); err != nil {
		return err
	}
	return s.terminalize(ctx, value, runCommitIdentity{segmentID: segmentID, commitID: commitID})
}

type runCommitIdentity struct {
	segmentID string
	commitID  string
}

func (s *RunStore) terminalize(
	ctx context.Context,
	value rundomain.Run,
	identity runCommitIdentity,
) error {
	return s.finish(ctx, "terminalize", value, identity, func(current rundomain.Run) (rundomain.Run, error) {
		outcome, terminal := value.Outcome()
		if !terminal {
			return rundomain.Run{}, errors.New("outcome is required")
		}
		failure, failed := value.Failure()
		var failureRef *rundomain.Failure
		if failed {
			failureRef = &failure
		}
		return current.Terminate(rundomain.Termination{
			Outcome: outcome, Detail: value.Detail(), Failure: failureRef,
			FinishedAt: value.FinishedAt(), MessageMark: value.MessageMark(),
		})
	})
}

// RunCommitCommitted proves that this exact immutable Application Run write-set crossed
// the durable boundary. It does not infer success from the coarse Run state:
// another Segment, restored/resumed Run, or later write attempt has a different
// or absent marker. Running markers require the same active Segment; waiting
// barriers and terminal boundaries retain the Segment that produced them.
// A command that starts and ends while already Waiting uses an empty Segment.
func (s *RunStore) RunCommitCommitted(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID string,
) (bool, error) {
	if segmentID == "" {
		if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
			return false, err
		}
	} else if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return false, err
	}
	var found int
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT count(*)
		   FROM runs
		  WHERE session_id = ? AND run_id = ?
		    AND commit_segment_id = ? AND commit_id = ?
		    AND ((state = ? AND active_segment_id = ?) OR state IN (?, ?))`,
		sessionID,
		runID,
		segmentID,
		commitID,
		runStateRunning,
		segmentID,
		runStateWaiting,
		runStateTerminal,
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("sqlite: verify Run commit %q: %w", commitID, err)
	}
	return found == 1, nil
}

func validateRunCommitIdentity(sessionID, runID, segmentID, commitID string) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session", value: sessionID},
		{name: "Run", value: runID},
		{name: "Segment", value: segmentID},
		{name: "commit", value: commitID},
	} {
		if strings.TrimSpace(identity.value) == "" || identity.value != strings.TrimSpace(identity.value) {
			return fmt.Errorf("sqlite: Run commit %s ID is required without surrounding whitespace", identity.name)
		}
	}
	return nil
}

func validateWaitingRunCommitIdentity(sessionID, runID, commitID string) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "session", value: sessionID},
		{name: "Run", value: runID},
		{name: "commit", value: commitID},
	} {
		if strings.TrimSpace(identity.value) == "" || identity.value != strings.TrimSpace(identity.value) {
			return fmt.Errorf("sqlite: waiting Run commit %s ID is required without surrounding whitespace", identity.name)
		}
	}
	return nil
}

// RebaseMessageMark applies an exact Application-decided coordinate rewrite to
// one terminal Run. Compaction does not change when the Run happened or any of
// its lifecycle facts, so updated_at deliberately remains untouched.
func (s *RunStore) RebaseMessageMark(ctx context.Context, expected, replacement rundomain.Run) error {
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: expected Run: %w", err)
	}
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: replacement Run: %w", err)
	}
	if !expected.State().IsTerminal() || !replacement.State().IsTerminal() {
		return errors.New("sqlite: rebase Run message watermark: terminal Run is required")
	}
	if expected.ID() != replacement.ID() || expected.SessionID() != replacement.SessionID() {
		return errors.New("sqlite: rebase Run message watermark changes identity")
	}
	derived, err := expected.WithMessageMark(replacement.MessageMark())
	if err != nil {
		return fmt.Errorf("sqlite: rebase Run message watermark: %w", err)
	}
	if !derived.Equal(replacement) {
		return errors.New("sqlite: rebase Run message watermark changes non-watermark facts")
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE runs SET message_mark = ?
		 WHERE session_id = ? AND run_id = ? AND state = ? AND message_mark = ?`,
		replacement.MessageMark(), expected.SessionID(), expected.ID(), runStateTerminal, expected.MessageMark(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: rebase Run %q message watermark: %w", expected.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Run %q message watermark rebase: %w", expected.ID(), err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: rebase Run %q message watermark lost its expected-value fence", expected.ID())
	}
	return nil
}

// RecoverLost ends the exact non-terminal Run whose executor state is no longer
// resumable. Unlike Terminalize, this recovery transition is legal from either
// Running or Waiting, because it describes a Run nobody is driving rather
// than one the executor finished.
func (s *RunStore) RecoverLost(ctx context.Context, value rundomain.Run) error {
	return s.finish(ctx, "recover lost", value, runCommitIdentity{}, func(current rundomain.Run) (rundomain.Run, error) {
		failure, failed := value.Failure()
		if !failed {
			return rundomain.Run{}, errors.New("lost failure is required")
		}
		return current.RecoverLost(failure, value.FinishedAt(), value.MessageMark())
	})
}

// finish ends a non-terminal Run, writing the terminal state, its reason, and the
// facts that explain it in ONE statement — a row can never claim a terminal
// state without the result behind it, nor hold a result while still running.
// transition invokes the aggregate's rule for this kind of ending; the UPDATE
// is a CAS on the committed source state, so a row that moved under the
// transaction fails instead of being overwritten.
func (s *RunStore) finish(
	ctx context.Context,
	op string,
	value rundomain.Run,
	commit runCommitIdentity,
	transition func(rundomain.Run) (rundomain.Run, error),
) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	failure, hasFailure := value.Failure()
	var failureRef *rundomain.Failure
	if hasFailure {
		failureRef = &failure
	}
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: %s run %q: %w", op, value.ID(), err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		current, found, err := s.runForTransition(ctx, value.ID())
		if err != nil {
			return err
		}
		if !found || current.SessionID() != value.SessionID() {
			return fmt.Errorf("sqlite: %s run: active run not found", op)
		}
		if commit.commitID != "" && current.ActiveSegmentID() != commit.segmentID {
			return fmt.Errorf(
				"sqlite: %s run: active Segment is %q, want %q",
				op,
				current.ActiveSegmentID(),
				commit.segmentID,
			)
		}
		current, err = current.AdvanceProgress(
			value.Metrics(), value.ContextTokens(), value.FinishedAt(),
		)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: advance aggregate metrics: %w", op, err)
		}
		next, err := transition(current)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		if !next.Equal(value) {
			return fmt.Errorf("sqlite: %s run: proposed Run differs from the aggregate transition", op)
		}
		outcome, _ := value.Outcome()
		query :=
			`UPDATE runs SET
			   state = ?, active_segment_id = '', commit_segment_id = ?, commit_id = ?,
			   outcome = ?, detail = ?, steps = ?, active_duration_ns = ?,
			   usage = ?, context_tokens = ?, problem = ?, message_mark = ?, finished_at = ?, updated_at = ?
			 WHERE session_id = ? AND run_id = ? AND state = ?`
		args := []any{
			coarseState(next.State()), commit.segmentID, commit.commitID,
			outcome.String(), value.Detail(), metrics.steps, metrics.durationNs,
			metrics.usage, next.ContextTokens(), encodedFailure,
			value.MessageMark(), value.FinishedAt().UTC().UnixNano(),
			runUpdatedAt(value), value.SessionID(), value.ID(), coarseState(current.State()),
		}
		if commit.commitID != "" {
			query += ` AND active_segment_id = ?`
			args = append(args, commit.segmentID)
		}
		res, err := conn(ctx, s.db).ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("sqlite: %s run: %w", op, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: %s run: read affected rows: %w", op, err)
		}
		if n == 0 {
			return fmt.Errorf("sqlite: %s run: state changed concurrently (was %s)", op, current.State())
		}
		// The Run's end is also a boundary of the session's Plan, and this CAS is
		// the only place a Run can reach terminal — so the boundary is stamped here
		// rather than by each caller that ends a Run, which is how "no terminal Run
		// without a recorded boundary" holds by construction. Restore is deliberately
		// NOT a boundary: an imported Run finished in another runtime, and stamping the
		// importing session's live list would invent a value that Run never had.
		return NewPlanStore(s.db).CaptureBoundary(ctx, value.SessionID(), value.ID())
	})
}

// Restore inserts a complete terminal Run row for a session being imported or
// restored. It is not an admission: an imported Run has already finished, so it
// never claims the session's non-terminal slot and never passes through the
// state machine. A non-terminal Run is refused — restoring one would hand the
// session's admission slot to an executor that is not running.
func (s *RunStore) Restore(ctx context.Context, value rundomain.Run) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	if !value.State().IsTerminal() {
		return fmt.Errorf("sqlite: restore run %q: state is %s, want terminal", value.ID(), value.State())
	}
	lineage := value.Lineage()
	if lineage.IsChild() {
		if err := s.validateChildPlacement(
			ctx,
			"restore",
			value.ID(),
			value.SessionID(),
			lineage.ParentRunID,
			lineage.RootRunID,
			false,
		); err != nil {
			return err
		}
	}
	metrics, err := runMetricsRow(value.Metrics())
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	failure, hasFailure := value.Failure()
	var failureRef *rundomain.Failure
	if hasFailure {
		failureRef = &failure
	}
	encodedFailure, err := encodeRunFailure(failureRef)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	capabilitiesOwner := value.Capabilities()
	if lineage.IsChild() {
		// A child materializes its root's capabilities on reads but owns no copy
		// on disk. The root row is the single durable author.
		capabilitiesOwner = rundomain.Capabilities{}
	}
	capabilities, err := encodeRunCapabilities(capabilitiesOwner)
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	outcome, _ := value.Outcome()
	selection := value.ModelSelection()
	limits := value.Limits()
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO runs(
		   run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
		   state, outcome, provider, model, goal_incarnation_id,
		   detail, steps, active_duration_ns, usage, context_tokens, problem,
		   max_total_tokens, max_steps, max_budget_usd,
		   capabilities, message_mark, started_at, finished_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID(), value.SessionID(),
		lineage.SpawnedByItemID, lineage.ParentRunID, lineage.RootRunID,
		coarseState(value.State()), outcome.String(),
		selection.Provider(), selection.Model(),
		value.GoalIncarnationID(),
		value.Detail(), metrics.steps, metrics.durationNs, metrics.usage, value.ContextTokens(), encodedFailure,
		limits.MaxTotalTokens, limits.MaxSteps, limits.MaxBudgetUSD, capabilities, value.MessageMark(),
		value.CreatedAt().UTC().UnixNano(), value.FinishedAt().UTC().UnixNano(), runUpdatedAt(value))
	if isPrimaryKeyViolation(err) {
		// A Run id belongs to one Session for its whole lifetime. An import that
		// would re-parent an existing Run is refused rather than silently taking it
		// over, which is what an upsert here would do.
		return fmt.Errorf("%w: run %q already exists", rundomain.ErrIdentityConflict, value.ID())
	}
	if err != nil {
		return fmt.Errorf("sqlite: restore run %q: %w", value.ID(), err)
	}
	return nil
}

// runForTransition reads the aggregate that a write is about to advance. It
// tolerates a temporarily absent Pending row because write-sets may delete that
// row before terminalizing the Run in the same transaction; the proposed
// aggregate transition remains the authority for whether the write is legal.
func (s *RunStore) runForTransition(ctx context.Context, runID string) (rundomain.Run, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id = ?`, runID)
	value, err := scanRunForRecovery(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return rundomain.Run{}, false, nil
	case err != nil:
		return rundomain.Run{}, false, fmt.Errorf("sqlite: read Run %q for transition: %w", runID, err)
	}
	return value, true, nil
}

// PageRuns returns one page of Runs a caller may browse, newest admission first,
// scoped to sessionID and statuses when provided. Descendants are excluded unless
// includeDescendants is true. It is the whole history, not the work in progress:
// a Run that ended is still the answer to "what did this session cost".
//
// The page is bounded here rather than by the caller. before is the (admission
// time, run id) position a previous page ended at, applied only when the run id
// is present — every anchor carries both halves, so an empty id is the first page
// and not a Run admitted at the epoch. The pair is what makes the order total,
// since two Runs in different sessions can be admitted in the same nanosecond.
//
// It returns whole Runs rather than a thinner admission projection, because the
// answer a caller renders includes what each Run has consumed — and a second Run
// shape assembled from a subset of the same columns would be a second answer to
// "what is this Run".
func (s *RunStore) PageRuns(ctx context.Context, sessionID string, statuses []rundomain.Status, includeDescendants bool, beforeStartedAt int64, beforeRunID string, limit int) ([]rundomain.Run, error) {
	query := `SELECT ` + runColumns + `
		 FROM runs AS r
		 ` + runReadJoins
	var args []any
	var conditions []string
	if !includeDescendants {
		conditions = append(conditions, `r.root_run_id = ''`)
	}
	if sessionID != "" {
		conditions = append(conditions, `r.session_id = ?`)
		args = append(args, sessionID)
	}
	if len(statuses) > 0 {
		columns, err := stateColumns(statuses)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		conditions = append(conditions, `r.state IN (`+placeholders(len(columns))+`)`)
		args = append(args, columns...)
	}
	if beforeRunID != "" {
		conditions = append(conditions, `(r.started_at < ? OR (r.started_at = ? AND r.run_id < ?))`)
		args = append(args, beforeStartedAt, beforeStartedAt, beforeRunID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY r.started_at DESC, r.run_id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	defer rows.Close()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page runs: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: page runs: %w", err)
	}
	return out, nil
}

// Run returns one Run by id alone, whatever state it is in. The session is not a
// parameter because a run id already identifies exactly one Run: making a caller
// supply the session too would mean it has to know where the Run lives before it
// can ask what the Run is.
func (s *RunStore) Run(ctx context.Context, runID string) (rundomain.Run, bool, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id = ?`, runID)
	run, err := scanRun(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return rundomain.Run{}, false, nil
	case err != nil:
		return rundomain.Run{}, false, fmt.Errorf("sqlite: read run %q: %w", runID, err)
	}
	return run, true, nil
}

// Tree resolves runID to its tree root and returns that root plus every
// descendant in one SQLite read. It deliberately makes no ordering promise:
// application/domain code validates the complete topology and derives canonical
// subtree order.
func (s *RunStore) Tree(ctx context.Context, runID string) ([]rundomain.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`WITH target AS (
		    SELECT CASE WHEN root_run_id = '' THEN run_id ELSE root_run_id END AS tree_root_id
		      FROM runs
		     WHERE run_id = ?
		 )
		 SELECT `+runColumns+`
		 FROM runs AS r
		 CROSS JOIN target
		 `+runReadJoins+`
		 WHERE r.run_id = target.tree_root_id OR r.root_run_id = target.tree_root_id`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
	}
	defer rows.Close()

	var runs []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read tree containing run %q: %w", runID, err)
	}
	return runs, nil
}

// RunsWithAncestors returns the Runs named by runIDs and every ancestor needed to
// connect them to their roots, in newest-admission order. It resolves the closure
// in one query without loading unrelated Runs from the Session.
func (s *RunStore) RunsWithAncestors(ctx context.Context, runIDs []string) ([]rundomain.Run, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(runIDs))
	for _, id := range runIDs {
		args = append(args, id)
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`WITH RECURSIVE lineage(run_id, parent_run_id) AS (
			SELECT run_id, parent_run_id
			  FROM runs
			 WHERE run_id IN (`+placeholders(len(runIDs))+`)
			UNION
			SELECT parent.run_id, parent.parent_run_id
			  FROM runs AS parent
			  JOIN lineage AS child ON child.parent_run_id = parent.run_id
		)
		 SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.run_id IN (SELECT run_id FROM lineage)
		 ORDER BY r.started_at DESC, r.run_id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
	}
	defer rows.Close()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read runs with ancestors: %w", err)
	}
	return out, nil
}

// stateColumns is the durable encoding of a status filter. An unrecognized status
// is refused rather than skipped: dropping it from the IN list would silently
// widen the page to statuses the caller did not ask for.
func stateColumns(statuses []rundomain.Status) ([]any, error) {
	out := make([]any, 0, len(statuses))
	for _, status := range statuses {
		if !status.Valid() {
			return nil, fmt.Errorf("unknown run status %d", status)
		}
		out = append(out, stateColumn(status))
	}
	return out, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// runColumns is the whole Run row, joined with the open interrupts a parked Run
// is waiting on — kept in the interrupts table so one park is one record.
const runColumns = `r.run_id, r.session_id, r.spawned_by_item_id, r.parent_run_id, r.root_run_id,
	r.state, r.active_segment_id, r.outcome,
	r.provider, r.model, r.goal_incarnation_id, r.detail,
	r.steps, r.active_duration_ns, r.usage, r.context_tokens, r.problem,
	r.max_total_tokens, r.max_steps, r.max_budget_usd, r.capabilities, tree_root.capabilities,
	r.message_mark, r.started_at, r.finished_at, r.updated_at, i.payload`

// runReadJoins materializes the root-owned capabilities and pending set for
// every Run in the tree. scanRun filters the aggregate payload by source Run ID,
// so a suspended sibling reads an empty direct-interrupt list rather than
// claiming another Run's questions.
const runReadJoins = `LEFT JOIN runs AS tree_root
		   ON tree_root.run_id = r.root_run_id AND tree_root.session_id = r.session_id
		 LEFT JOIN interrupts AS i
		   ON i.root_run_id = CASE
		        WHEN r.root_run_id = '' THEN r.run_id
		        ELSE r.root_run_id
		      END
		  AND i.session_id = r.session_id`

// ListRuns returns a session's Runs in admission order, each as the complete
// aggregate: its lifecycle position, the facts it accrued, and — while parked —
// the interrupts it is waiting on.
func (s *RunStore) ListRuns(ctx context.Context, sessionID string) ([]rundomain.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		 FROM runs AS r
		 `+runReadJoins+`
		 WHERE r.session_id = ? ORDER BY r.started_at, r.run_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	defer rows.Close()

	var out []rundomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list runs: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list runs: %w", err)
	}
	return out, nil
}

// coarseState is the column value a Run in state s is stored under. It routes
// through the domain's lifecycle position so a row written by Suspend and a query
// filtering on [rundomain.StatusWaiting] cannot disagree about which value that
// is — the partial unique index keys on non-terminal, so every terminal State
// collapses to the one 'terminal' value (the fine reason lives in runs.outcome).
func coarseState(s rundomain.State) string {
	return stateColumn(s.Status())
}

// stateColumn is the durable spelling of a lifecycle position. It stays an
// explicit table rather than [rundomain.Status.String]: these three strings are
// on disk and inside the partial unique index's predicate, so a Go rename must not
// be able to rewrite them.
func stateColumn(status rundomain.Status) string {
	switch status {
	case rundomain.StatusWaiting:
		return runStateWaiting
	case rundomain.StatusFinished:
		return runStateTerminal
	default:
		return runStateRunning
	}
}

// Delete drops one Run's row. The rollback boundary uses it: a Run being dropped
// wholesale frees the session's admission slot by ceasing to exist, so there is
// nothing left to terminalize.
func (s *RunStore) Delete(ctx context.Context, sessionID, runID string) error {
	if sessionID == "" || runID == "" {
		return errors.New("sqlite: delete run requires sessionId + runId")
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM runs WHERE run_id = ? AND session_id = ?`, runID, sessionID,
	); err != nil {
		return fmt.Errorf("sqlite: delete run: %w", err)
	}
	return nil
}

// DeleteForSession drops every Run row of a session whose durable state is being
// removed or replaced wholesale — the session-delete cascade, the import/restore
// replace, and the child-Run subtree purge. Freeing the admission slot by deletion
// (not terminalization) keeps the runs table from accumulating dead rows for
// sessions that no longer exist. Joins the caller's transaction via the context.
func (s *RunStore) DeleteForSession(ctx context.Context, sessionID string) error {
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM runs WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: delete runs for session: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-index failure — for
// this table, the partial-unique-index rejection that means the session already
// holds a non-terminal run. A primary-key collision has its OWN extended code and
// does NOT appear here, which is what lets an id clash be told apart from a busy
// session. modernc.org/sqlite surfaces both as a typed *sqlite.Error carrying the
// extended result code.
func isUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE
}

// isPrimaryKeyViolation reports whether err is a SQLite PRIMARY KEY collision —
// here, a run id that already belongs to a Run.
func isPrimaryKeyViolation(err error) bool {
	se, ok := errors.AsType[*sqlite3.Error](err)
	return ok && se.Code() == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY
}

// runUpdatedAt is the row's touch time. A caller that recorded one is honored so
// a restored Run keeps the timestamp it was exported with; otherwise the write
// stamps itself.
func runUpdatedAt(run rundomain.Run) int64 {
	if run.UpdatedAt().IsZero() {
		return time.Now().UTC().UnixNano()
	}
	return run.UpdatedAt().UTC().UnixNano()
}
