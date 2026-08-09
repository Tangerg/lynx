package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// RecoveryStore exposes durable application facts and atomically applies the
// recovery plan derived from them. It never validates executor payloads or
// decides which Run tree survives.
type RecoveryStore interface {
	ListNonTerminalRuns(ctx context.Context) ([]transcript.Run, error)
	ListPendingInterrupts(ctx context.Context) ([]Pending, error)
	SessionByID(ctx context.Context, sessionID string) (session.Session, error)
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error)
	CountMessages(ctx context.Context, sessionID string) (int, error)
	CommitRecovery(ctx context.Context, commit RecoveryCommit) error
}

// CheckpointResumability is the recovery use case's narrow checkpoint probe.
// false, nil means the opaque continuation is unavailable or incompatible;
// an error means validation itself failed and startup must stop without writes.
type CheckpointResumability interface {
	CanResumeCheckpoint(ctx context.Context, expected ExecutorCheckpointExpectation) (bool, error)
}

// RecoveryCommit is the complete atomic write-set for boot reconciliation.
// LostRuns are ordered child-before-parent. PreservedCheckpointRootIDs is the
// exact owner set; every other checkpoint aggregate is deleted.
type RecoveryCommit struct {
	LostRuns                   []transcript.Run
	ItemReplacements           []ItemReplacement
	GoalRuns                   []goal.RunRecord
	DeleteInterrupts           []InterruptOwner
	PreservedCheckpointRootIDs []string
}

// InterruptOwner is the complete mutation authority for one root-owned
// interrupt record. Recovery names every lost root, including a record hidden
// in the resuming state after an answer claim; storage deletion is idempotent.
type InterruptOwner struct {
	SessionID string
	RootRunID string
}

// Recovery owns the application policy that reconciles Run trees abandoned by
// a previous process. Construction happens before new Run admission, so its
// read/validate phase observes an exclusive boot snapshot; CommitRecovery still
// applies the resulting write-set atomically and Run transitions remain CAS
// guarded by storage.
type Recovery struct {
	store       RecoveryStore
	checkpoints CheckpointResumability
	now         func() time.Time
}

// recoveryPlanner owns one boot reconciliation snapshot and the caches needed
// to derive its atomic write-set. It is intentionally Application-private:
// deciding whether an opaque checkpoint preserves a product Run is a recovery
// policy, not a Run aggregate or executor concern.
type recoveryPlanner struct {
	ctx           context.Context
	store         RecoveryStore
	checkpoints   CheckpointResumability
	pending       []Pending
	pendingByRoot map[string]Pending
	trees         map[string]recoveryRunTree
	transcripts   map[string][]transcript.Item
	sessions      map[string]session.Session
	messageMarks  map[string]int
	preserved     map[string]struct{}
	commit        RecoveryCommit
	finishedAt    time.Time
	reconciled    int
}

// NewRecovery constructs the boot recovery use case.
func NewRecovery(store RecoveryStore, checkpoints CheckpointResumability) (*Recovery, error) {
	if store == nil {
		return nil, errors.New("runs: recovery store is required")
	}
	if checkpoints == nil {
		return nil, errors.New("runs: checkpoint resumability is required")
	}
	return &Recovery{store: store, checkpoints: checkpoints, now: time.Now}, nil
}

// Reconcile preserves only complete waiting trees whose durable hand-off
// and opaque executor checkpoint remain coherent. Every other non-terminal tree
// is recovered as run_lost in one application transaction.
func (r *Recovery) Reconcile(ctx context.Context) (int, error) {
	planner, err := newRecoveryPlanner(ctx, r)
	if err != nil {
		return 0, err
	}
	commit, reconciled, err := planner.plan()
	if err != nil {
		return 0, err
	}
	if err := r.store.CommitRecovery(ctx, commit); err != nil {
		return 0, fmt.Errorf("runs: commit boot recovery: %w", err)
	}
	return reconciled, nil
}

func newRecoveryPlanner(ctx context.Context, recovery *Recovery) (*recoveryPlanner, error) {
	if recovery == nil {
		return nil, errors.New("runs: recovery use case is required")
	}
	active, err := recovery.store.ListNonTerminalRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load non-terminal Runs for recovery: %w", err)
	}
	pending, err := recovery.store.ListPendingInterrupts(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load pending interrupts for recovery: %w", err)
	}

	pendingByRun := make(map[string]Pending, len(pending))
	checkpointOwners := make(map[string]string, len(pending))
	for _, open := range pending {
		if _, duplicate := pendingByRun[open.RootRunID]; duplicate {
			return nil, fmt.Errorf("runs: recovery has duplicate Pending for root Run %q", open.RootRunID)
		}
		root, ok := open.RootContinuation()
		if !ok {
			return nil, fmt.Errorf("runs: recovery interrupt %q has no root continuation", open.RootRunID)
		}
		if owner, duplicate := checkpointOwners[root.MemberID]; duplicate {
			return nil, fmt.Errorf(
				"runs: recovery checkpoint %q is owned by interrupts %q and %q",
				root.MemberID,
				owner,
				open.RootRunID,
			)
		}
		checkpointOwners[root.MemberID] = open.RootRunID
		pendingByRun[open.RootRunID] = open
	}

	trees, err := groupRecoveryRunTrees(active)
	if err != nil {
		return nil, err
	}
	return &recoveryPlanner{
		ctx:           ctx,
		store:         recovery.store,
		checkpoints:   recovery.checkpoints,
		pending:       slices.Clone(pending),
		pendingByRoot: pendingByRun,
		trees:         trees,
		transcripts:   make(map[string][]transcript.Item),
		sessions:      make(map[string]session.Session),
		messageMarks:  make(map[string]int),
		preserved:     make(map[string]struct{}, len(trees)),
		finishedAt:    recovery.now().UTC(),
	}, nil
}

func (planner *recoveryPlanner) plan() (RecoveryCommit, int, error) {
	rootRunIDs := make([]string, 0, len(planner.trees))
	for rootRunID := range planner.trees {
		rootRunIDs = append(rootRunIDs, rootRunID)
	}
	slices.Sort(rootRunIDs)
	for _, rootRunID := range rootRunIDs {
		if err := planner.planTree(rootRunID); err != nil {
			return RecoveryCommit{}, 0, err
		}
	}
	for _, open := range planner.pending {
		if _, preserved := planner.preserved[open.RootRunID]; preserved {
			root, _ := open.RootContinuation()
			planner.commit.PreservedCheckpointRootIDs = append(
				planner.commit.PreservedCheckpointRootIDs,
				root.MemberID,
			)
		}
	}
	slices.SortFunc(planner.commit.DeleteInterrupts, func(left, right InterruptOwner) int {
		if bySession := strings.Compare(left.SessionID, right.SessionID); bySession != 0 {
			return bySession
		}
		return strings.Compare(left.RootRunID, right.RootRunID)
	})
	slices.Sort(planner.commit.PreservedCheckpointRootIDs)
	if err := planner.commit.Validate(); err != nil {
		return RecoveryCommit{}, 0, err
	}
	return planner.commit, planner.reconciled, nil
}

func (planner *recoveryPlanner) planTree(rootRunID string) error {
	tree := planner.trees[rootRunID]
	items, err := planner.transcript(tree.root.SessionID)
	if err != nil {
		return err
	}
	open, hasInterrupt := planner.pendingByRoot[rootRunID]
	if tree.root.State == rundomain.Waiting && hasInterrupt {
		sess, err := planner.session(tree.root.SessionID)
		if err != nil {
			return err
		}
		resumable, err := validateRecoveryParkedTree(
			planner.ctx,
			tree,
			open,
			sess,
			items,
			planner.checkpoints,
		)
		if err != nil {
			return err
		}
		if resumable {
			planner.preserved[rootRunID] = struct{}{}
			return nil
		}
	}
	messageMark, err := planner.messageMark(tree.root.SessionID)
	if err != nil {
		return err
	}
	lostRuns, replacements, err := recoverLostTree(tree, items, messageMark, planner.finishedAt)
	if err != nil {
		return err
	}
	planner.commit.LostRuns = append(planner.commit.LostRuns, lostRuns...)
	planner.commit.ItemReplacements = append(planner.commit.ItemReplacements, replacements...)
	planner.commit.DeleteInterrupts = append(planner.commit.DeleteInterrupts, InterruptOwner{
		SessionID: tree.root.SessionID,
		RootRunID: tree.root.ID,
	})
	if tree.root.GoalLeaseID != "" {
		record, err := recoveredGoalRun(tree.root.ID, lostRuns)
		if err != nil {
			return err
		}
		planner.commit.GoalRuns = append(planner.commit.GoalRuns, record)
	}
	planner.reconciled += len(lostRuns)
	return nil
}

func (planner *recoveryPlanner) transcript(sessionID string) ([]transcript.Item, error) {
	if items, ok := planner.transcripts[sessionID]; ok {
		return items, nil
	}
	items, err := planner.store.ListTranscript(planner.ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runs: load recovery transcript for Session %q: %w", sessionID, err)
	}
	planner.transcripts[sessionID] = items
	return items, nil
}

func (planner *recoveryPlanner) session(sessionID string) (session.Session, error) {
	if sess, ok := planner.sessions[sessionID]; ok {
		return sess, nil
	}
	sess, err := planner.store.SessionByID(planner.ctx, sessionID)
	if err != nil {
		return session.Session{}, fmt.Errorf("runs: load recovery Session %q: %w", sessionID, err)
	}
	if sess.ID != sessionID {
		return session.Session{}, fmt.Errorf(
			"runs: recovery Session lookup for %q returned %q",
			sessionID,
			sess.ID,
		)
	}
	planner.sessions[sessionID] = sess
	return sess, nil
}

func (planner *recoveryPlanner) messageMark(sessionID string) (int, error) {
	if mark, ok := planner.messageMarks[sessionID]; ok {
		return mark, nil
	}
	mark, err := planner.store.CountMessages(planner.ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("runs: load recovery message watermark for Session %q: %w", sessionID, err)
	}
	planner.messageMarks[sessionID] = mark
	return mark, nil
}

func recoveredGoalRun(rootRunID string, lostRuns []transcript.Run) (goal.RunRecord, error) {
	if len(lostRuns) == 0 {
		return goal.RunRecord{}, fmt.Errorf("runs: recovered tree %q has no terminal root", rootRunID)
	}
	lostRoot := lostRuns[len(lostRuns)-1]
	if lostRoot.ID != rootRunID || lostRoot.Outcome == nil {
		return goal.RunRecord{}, fmt.Errorf("runs: recovered tree %q has no terminal root", rootRunID)
	}
	record := goal.RunRecord{
		SessionID:   lostRoot.SessionID,
		LeaseID:     lostRoot.GoalLeaseID,
		RunID:       lostRoot.ID,
		Outcome:     *lostRoot.Outcome,
		Steps:       lostRoot.Metrics.Steps,
		CompletedAt: lostRoot.FinishedAt,
	}
	if lostRoot.Metrics.Usage != nil && lostRoot.Metrics.Usage.CostUSD != nil {
		record.CostUSD = *lostRoot.Metrics.Usage.CostUSD
	}
	return record, nil
}

type recoveryRunTree struct {
	root      transcript.Run
	runsByID  map[string]transcript.Run
	postorder []string
}

func groupRecoveryRunTrees(active []transcript.Run) (map[string]recoveryRunTree, error) {
	grouped := make(map[string][]transcript.Run)
	for index, run := range active {
		if err := run.Validate(); err != nil {
			return nil, fmt.Errorf("runs: validate recovery Run[%d] %q: %w", index, run.ID, err)
		}
		rootRunID := run.Lineage().TreeRootID(run.ID)
		grouped[rootRunID] = append(grouped[rootRunID], run)
	}

	trees := make(map[string]recoveryRunTree, len(grouped))
	for rootRunID, runs := range grouped {
		members := make([]rundomain.TreeMember, 0, len(runs))
		runsByID := make(map[string]transcript.Run, len(runs))
		for _, run := range runs {
			members = append(members, rundomain.TreeMember{RunID: run.ID, Lineage: run.Lineage()})
			runsByID[run.ID] = run
		}
		topology, err := rundomain.NewTree(rootRunID, members)
		if err != nil {
			return nil, fmt.Errorf("runs: assemble recovery Run tree %q: %w", rootRunID, err)
		}
		root, found := runsByID[rootRunID]
		if !found {
			return nil, fmt.Errorf("runs: assemble recovery Run tree %q: root is missing", rootRunID)
		}
		for _, run := range runs {
			if run.SessionID != root.SessionID {
				return nil, fmt.Errorf(
					"runs: recovery Run %q belongs to Session %q, want tree Session %q",
					run.ID,
					run.SessionID,
					root.SessionID,
				)
			}
		}
		trees[rootRunID] = recoveryRunTree{root: root, runsByID: runsByID, postorder: topology.Postorder()}
	}
	return trees, nil
}

func recoverLostTree(
	tree recoveryRunTree,
	items []transcript.Item,
	messageMark int,
	finishedAt time.Time,
) ([]transcript.Run, []ItemReplacement, error) {
	lostRuns := make([]transcript.Run, 0, len(tree.postorder))
	var replacements []ItemReplacement
	for _, runID := range tree.postorder {
		active := tree.runsByID[runID]
		for _, item := range items {
			if item.RunID != active.ID || item.Status != transcript.ItemRunning {
				continue
			}
			replacement := item
			replacement.Status = transcript.ItemIncomplete
			if replacement.Kind == transcript.ToolCall {
				replacement.FinishedAt = finishedAt.UTC()
				replacement.Error = &transcript.Problem{
					Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem,
					Detail: "tool call interrupted because the run was lost on restart",
				}
			}
			replacements = append(replacements, ItemReplacement{Expected: item, Replacement: replacement})
		}

		next, ok := active.State.RecoverLost()
		if !ok {
			return nil, nil, fmt.Errorf("runs: recover lost Run %q: state %s is not recoverable", active.ID, active.State)
		}
		outcome := rundomain.OutcomeLost
		active.State = next
		active.ActiveSegmentID = ""
		active.Outcome = &outcome
		active.Detail = ""
		active.Error = &transcript.Problem{
			Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
			Detail: "run lost on restart",
		}
		active.Interrupts = nil
		active.FinishedAt = finishedAt
		active.UpdatedAt = finishedAt
		active.MessageMark = messageMark
		lostRuns = append(lostRuns, active)
	}
	return lostRuns, replacements, nil
}
