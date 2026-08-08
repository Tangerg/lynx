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
	DeletePending              []PendingDeletion
	PreservedCheckpointRootIDs []string
}

// PendingDeletion is the owner-bound identity of one stale barrier removed by
// boot recovery. A root Run ID alone is not mutation authority.
type PendingDeletion struct {
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
	active, err := r.store.ListNonTerminalRuns(ctx)
	if err != nil {
		return 0, fmt.Errorf("runs: load non-terminal Runs for recovery: %w", err)
	}
	pending, err := r.store.ListPendingInterrupts(ctx)
	if err != nil {
		return 0, fmt.Errorf("runs: load pending interrupts for recovery: %w", err)
	}

	pendingByRun := make(map[string]Pending, len(pending))
	checkpointOwners := make(map[string]string, len(pending))
	for _, open := range pending {
		if _, duplicate := pendingByRun[open.RootRunID]; duplicate {
			return 0, fmt.Errorf("runs: recovery has duplicate Pending for root Run %q", open.RootRunID)
		}
		root, ok := open.RootContinuation()
		if !ok {
			return 0, fmt.Errorf("runs: recovery interrupt %q has no root continuation", open.RootRunID)
		}
		if owner, duplicate := checkpointOwners[root.MemberID]; duplicate {
			return 0, fmt.Errorf(
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
		return 0, err
	}
	rootRunIDs := make([]string, 0, len(trees))
	for rootRunID := range trees {
		rootRunIDs = append(rootRunIDs, rootRunID)
	}
	slices.Sort(rootRunIDs)

	transcripts := make(map[string][]transcript.Item)
	sessions := make(map[string]session.Session)
	messageMarks := make(map[string]int)
	preservedRuns := make(map[string]struct{}, len(trees))
	commit := RecoveryCommit{}
	finishedAt := r.now().UTC()
	reconciled := 0
	for _, rootRunID := range rootRunIDs {
		tree := trees[rootRunID]
		items, ok := transcripts[tree.root.SessionID]
		if !ok {
			items, err = r.store.ListTranscript(ctx, tree.root.SessionID)
			if err != nil {
				return 0, fmt.Errorf("runs: load recovery transcript for Session %q: %w", tree.root.SessionID, err)
			}
			transcripts[tree.root.SessionID] = items
		}

		open, hasInterrupt := pendingByRun[rootRunID]
		if tree.root.State == rundomain.Waiting && hasInterrupt {
			sess, ok := sessions[tree.root.SessionID]
			if !ok {
				sess, err = r.store.SessionByID(ctx, tree.root.SessionID)
				if err != nil {
					return 0, fmt.Errorf("runs: load recovery Session %q: %w", tree.root.SessionID, err)
				}
				if sess.ID != tree.root.SessionID {
					return 0, fmt.Errorf(
						"runs: recovery Session lookup for %q returned %q",
						tree.root.SessionID,
						sess.ID,
					)
				}
				sessions[tree.root.SessionID] = sess
			}
			resumable, err := validateRecoveryParkedTree(ctx, tree, open, sess, items, r.checkpoints)
			if err != nil {
				return 0, err
			}
			if resumable {
				preservedRuns[rootRunID] = struct{}{}
				continue
			}
		}

		messageMark, ok := messageMarks[tree.root.SessionID]
		if !ok {
			messageMark, err = r.store.CountMessages(ctx, tree.root.SessionID)
			if err != nil {
				return 0, fmt.Errorf("runs: load recovery message watermark for Session %q: %w", tree.root.SessionID, err)
			}
			messageMarks[tree.root.SessionID] = messageMark
		}
		lostRuns, replacements, err := recoverLostTree(tree, items, messageMark, finishedAt)
		if err != nil {
			return 0, err
		}
		commit.LostRuns = append(commit.LostRuns, lostRuns...)
		commit.ItemReplacements = append(commit.ItemReplacements, replacements...)
		if tree.root.GoalLeaseID != "" {
			lostRoot := lostRuns[len(lostRuns)-1]
			if lostRoot.ID != tree.root.ID || lostRoot.Outcome == nil {
				return 0, fmt.Errorf("runs: recovered tree %q has no terminal root", tree.root.ID)
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
			commit.GoalRuns = append(commit.GoalRuns, record)
		}
		reconciled += len(lostRuns)
	}

	for _, open := range pending {
		if _, preserved := preservedRuns[open.RootRunID]; preserved {
			root, _ := open.RootContinuation()
			commit.PreservedCheckpointRootIDs = append(commit.PreservedCheckpointRootIDs, root.MemberID)
			continue
		}
		commit.DeletePending = append(commit.DeletePending, PendingDeletion{
			SessionID: open.SessionID,
			RootRunID: open.RootRunID,
		})
	}
	slices.SortFunc(commit.DeletePending, func(left, right PendingDeletion) int {
		if bySession := strings.Compare(left.SessionID, right.SessionID); bySession != 0 {
			return bySession
		}
		return strings.Compare(left.RootRunID, right.RootRunID)
	})
	slices.Sort(commit.PreservedCheckpointRootIDs)
	if err := commit.Validate(); err != nil {
		return 0, err
	}
	if err := r.store.CommitRecovery(ctx, commit); err != nil {
		return 0, fmt.Errorf("runs: commit boot recovery: %w", err)
	}
	return reconciled, nil
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
		members := make([]rundomain.RunTreeMember, 0, len(runs))
		runsByID := make(map[string]transcript.Run, len(runs))
		for _, run := range runs {
			members = append(members, rundomain.RunTreeMember{RunID: run.ID, Lineage: run.Lineage()})
			runsByID[run.ID] = run
		}
		topology, err := rundomain.NewRunTree(rootRunID, members)
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
