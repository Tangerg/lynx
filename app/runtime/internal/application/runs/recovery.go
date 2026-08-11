package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

const recoveryLostToolResult = "tool result unavailable because execution state was lost"

// RecoveryStore exposes durable application facts and atomically applies the
// recovery plan derived from them. It never validates executor payloads or
// decides which Run tree survives.
type RecoveryStore interface {
	ListNonTerminalRuns(ctx context.Context) ([]rundomain.Run, error)
	ListPendingInterrupts(ctx context.Context) ([]Pending, error)
	ListOpenModelInvocations(ctx context.Context) ([]OpenModelInvocation, error)
	ListOpenToolInvocations(ctx context.Context) ([]OpenToolInvocation, error)
	SessionByID(ctx context.Context, sessionID string) (session.Session, error)
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, error)
	LoadExecutorCheckpoint(ctx context.Context, rootMemberID string) (ExecutorCheckpoint, error)
	ReadMessages(ctx context.Context, sessionID string) ([]corechat.Message, error)
	CountMessages(ctx context.Context, sessionID string) (int, error)
	CommitRecovery(ctx context.Context, commit RecoveryCommit) error
}

// WaitingExecutionResumability is the recovery use case's narrow executor
// probe. The Application supplies the exact durable continuation; false, nil
// means its opaque state is incompatible or indeterminate, while an error means
// the probe itself was inconclusive and startup must stop without writes.
type WaitingExecutionResumability interface {
	CanResumeWaitingExecution(ctx context.Context, continuation WaitingContinuation) (bool, error)
}

// RecoveryCommit is the complete atomic write-set for boot reconciliation.
// LostRuns are ordered child-before-parent. PreservedCheckpointRootIDs is the
// exact owner set; every other checkpoint aggregate is deleted.
type RecoveryCommit struct {
	LostRuns                   []rundomain.Run
	ItemReplacements           []ItemReplacement
	ConversationTransitions    []RecoveryConversationTransition
	ModelInvocations           []ModelInvocationRecovery
	ToolInvocations            []ToolInvocationRecovery
	GoalRuns                   []goal.RunRecord
	DeleteInterrupts           []InterruptOwner
	PreservedCheckpointRootIDs []string
}

// OpenModelInvocation is an operational provider attempt that crossed the
// external boundary but has no durable terminal observation. Because boot
// recovery runs before executor admission, no live process can still own it.
type OpenModelInvocation struct {
	SessionID string
	RunID     string
	SegmentID string
	CallID    string
	StartedAt time.Time
}

// OpenToolInvocation is an operational Tool attempt without a durable
// terminal observation. ItemID binds the attempt back to its canonical
// Transcript lifecycle without copying arguments or results into the journal.
type OpenToolInvocation struct {
	SessionID string
	RunID     string
	SegmentID string
	CallID    string
	ItemID    string
	StartedAt time.Time
}

// ModelInvocationRecovery marks one boot-abandoned provider attempt unknown.
// The state is implied by the recovery operation rather than stored as an
// application enum in the persistence port.
type ModelInvocationRecovery struct {
	SessionID  string
	RunID      string
	SegmentID  string
	CallID     string
	StartedAt  time.Time
	FinishedAt time.Time
}

// ToolInvocationRecovery marks one boot-abandoned Tool attempt incomplete.
type ToolInvocationRecovery struct {
	SessionID  string
	RunID      string
	SegmentID  string
	CallID     string
	ItemID     string
	StartedAt  time.Time
	FinishedAt time.Time
}

// RecoveryConversationTransition closes the model context for one lost Run
// tree. ExpectedCount is the boot snapshot's durable watermark; Messages is
// empty when the context was already closed, or one Tool message containing an
// error result for every unresolved provider ToolCall.
type RecoveryConversationTransition struct {
	RootRunID     string
	SessionID     string
	ExpectedCount int
	Messages      []corechat.Message
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
	store        RecoveryStore
	resumability WaitingExecutionResumability
	now          func() time.Time
}

// recoveryPlanner owns one boot reconciliation snapshot and the caches needed
// to derive its atomic write-set. It is intentionally Application-private:
// deciding whether an opaque checkpoint preserves a product Run is a recovery
// policy, not a Run aggregate or executor concern.
type recoveryPlanner struct {
	ctx           context.Context
	store         RecoveryStore
	resumability  WaitingExecutionResumability
	pending       []Pending
	pendingByRoot map[string]Pending
	trees         map[string]recoveryRunTree
	transcripts   map[string][]transcript.Item
	sessions      map[string]session.Session
	conversations map[string]recoveryConversationSnapshot
	preserved     map[string]struct{}
	commit        RecoveryCommit
	finishedAt    time.Time
	reconciled    int
}

type recoveryConversationSnapshot struct {
	history conversation.Conversation
	count   int
}

// NewRecovery constructs the boot recovery use case.
func NewRecovery(store RecoveryStore, resumability WaitingExecutionResumability) (*Recovery, error) {
	if store == nil {
		return nil, errors.New("runs: recovery store is required")
	}
	if resumability == nil {
		return nil, errors.New("runs: waiting execution resumability is required")
	}
	return &Recovery{store: store, resumability: resumability, now: time.Now}, nil
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
	modelInvocations, err := recovery.store.ListOpenModelInvocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load open model invocations for recovery: %w", err)
	}
	toolInvocations, err := recovery.store.ListOpenToolInvocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load open Tool invocations for recovery: %w", err)
	}
	finishedAt := recovery.now().UTC()

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
	planner := &recoveryPlanner{
		ctx:           ctx,
		store:         recovery.store,
		resumability:  recovery.resumability,
		pending:       slices.Clone(pending),
		pendingByRoot: pendingByRun,
		trees:         trees,
		transcripts:   make(map[string][]transcript.Item),
		sessions:      make(map[string]session.Session),
		conversations: make(map[string]recoveryConversationSnapshot),
		preserved:     make(map[string]struct{}, len(trees)),
		finishedAt:    finishedAt,
	}
	for _, invocation := range modelInvocations {
		planner.commit.ModelInvocations = append(planner.commit.ModelInvocations, ModelInvocationRecovery{
			SessionID: invocation.SessionID, RunID: invocation.RunID, SegmentID: invocation.SegmentID,
			CallID: invocation.CallID, StartedAt: invocation.StartedAt, FinishedAt: finishedAt,
		})
	}
	for _, invocation := range toolInvocations {
		planner.commit.ToolInvocations = append(planner.commit.ToolInvocations, ToolInvocationRecovery{
			SessionID: invocation.SessionID, RunID: invocation.RunID, SegmentID: invocation.SegmentID,
			CallID: invocation.CallID, ItemID: invocation.ItemID,
			StartedAt: invocation.StartedAt, FinishedAt: finishedAt,
		})
	}
	return planner, nil
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
	slices.SortFunc(planner.commit.ModelInvocations, compareModelInvocationRecoveries)
	slices.SortFunc(planner.commit.ToolInvocations, compareToolInvocationRecoveries)
	slices.Sort(planner.commit.PreservedCheckpointRootIDs)
	if err := planner.commit.Validate(); err != nil {
		return RecoveryCommit{}, 0, err
	}
	return planner.commit, planner.reconciled, nil
}

func compareModelInvocationRecoveries(left, right ModelInvocationRecovery) int {
	for _, comparison := range []int{
		strings.Compare(left.SessionID, right.SessionID),
		strings.Compare(left.RunID, right.RunID),
		strings.Compare(left.SegmentID, right.SegmentID),
		strings.Compare(left.CallID, right.CallID),
	} {
		if comparison != 0 {
			return comparison
		}
	}
	return 0
}

func compareToolInvocationRecoveries(left, right ToolInvocationRecovery) int {
	if comparison := compareModelInvocationRecoveries(
		ModelInvocationRecovery{
			SessionID: left.SessionID, RunID: left.RunID, SegmentID: left.SegmentID, CallID: left.CallID,
		},
		ModelInvocationRecovery{
			SessionID: right.SessionID, RunID: right.RunID, SegmentID: right.SegmentID, CallID: right.CallID,
		},
	); comparison != 0 {
		return comparison
	}
	return strings.Compare(left.ItemID, right.ItemID)
}

func (planner *recoveryPlanner) planTree(rootRunID string) error {
	tree := planner.trees[rootRunID]
	items, err := planner.transcript(tree.root.SessionID())
	if err != nil {
		return err
	}
	open, hasInterrupt := planner.pendingByRoot[rootRunID]
	if tree.root.State() == rundomain.Waiting && hasInterrupt {
		sess, err := planner.session(tree.root.SessionID())
		if err != nil {
			return err
		}
		resumable, err := validateRecoveryParkedTree(
			planner.ctx,
			tree,
			open,
			sess,
			items,
			planner.store,
			planner.resumability,
		)
		if err != nil {
			return err
		}
		if resumable {
			planner.preserved[rootRunID] = struct{}{}
			return nil
		}
	}
	conversationSnapshot, err := planner.conversation(tree.root.SessionID())
	if err != nil {
		return err
	}
	_, closure, err := conversationSnapshot.history.CloseOpenToolCalls(recoveryLostToolResult)
	if err != nil {
		return fmt.Errorf(
			"runs: close recovery conversation for root Run %q: %w",
			tree.root.ID(),
			err,
		)
	}
	messageMark := conversationSnapshot.count + len(closure)
	lostRuns, replacements, err := recoverLostTree(tree, items, messageMark, planner.finishedAt)
	if err != nil {
		return err
	}
	planner.commit.LostRuns = append(planner.commit.LostRuns, lostRuns...)
	planner.commit.ItemReplacements = append(planner.commit.ItemReplacements, replacements...)
	planner.commit.ConversationTransitions = append(
		planner.commit.ConversationTransitions,
		RecoveryConversationTransition{
			RootRunID: tree.root.ID(), SessionID: tree.root.SessionID(),
			ExpectedCount: conversationSnapshot.count, Messages: closure,
		},
	)
	planner.commit.DeleteInterrupts = append(planner.commit.DeleteInterrupts, InterruptOwner{
		SessionID: tree.root.SessionID(),
		RootRunID: tree.root.ID(),
	})
	if tree.root.GoalIncarnationID() != "" {
		record, err := recoveredGoalRun(tree.root.ID(), lostRuns)
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
	if sess.ID() != sessionID {
		return session.Session{}, fmt.Errorf(
			"runs: recovery Session lookup for %q returned %q",
			sessionID,
			sess.ID(),
		)
	}
	planner.sessions[sessionID] = sess
	return sess, nil
}

func (planner *recoveryPlanner) conversation(sessionID string) (recoveryConversationSnapshot, error) {
	if snapshot, ok := planner.conversations[sessionID]; ok {
		return snapshot, nil
	}
	messages, err := planner.store.ReadMessages(planner.ctx, sessionID)
	if err != nil {
		return recoveryConversationSnapshot{}, fmt.Errorf(
			"runs: load recovery conversation for Session %q: %w",
			sessionID,
			err,
		)
	}
	history, err := conversation.New(messages)
	if err != nil {
		return recoveryConversationSnapshot{}, fmt.Errorf(
			"runs: validate recovery conversation for Session %q: %w",
			sessionID,
			err,
		)
	}
	count, err := planner.store.CountMessages(planner.ctx, sessionID)
	if err != nil {
		return recoveryConversationSnapshot{}, fmt.Errorf(
			"runs: load recovery message watermark for Session %q: %w",
			sessionID,
			err,
		)
	}
	if count != history.Count() {
		return recoveryConversationSnapshot{}, fmt.Errorf(
			"runs: recovery conversation for Session %q decoded %d messages at stored watermark %d",
			sessionID,
			history.Count(),
			count,
		)
	}
	snapshot := recoveryConversationSnapshot{history: history, count: count}
	planner.conversations[sessionID] = snapshot
	return snapshot, nil
}

func recoveredGoalRun(rootRunID string, lostRuns []rundomain.Run) (goal.RunRecord, error) {
	if len(lostRuns) == 0 {
		return goal.RunRecord{}, fmt.Errorf("runs: recovered tree %q has no terminal root", rootRunID)
	}
	lostRoot := lostRuns[len(lostRuns)-1]
	outcome, terminal := lostRoot.Outcome()
	if lostRoot.ID() != rootRunID || !terminal {
		return goal.RunRecord{}, fmt.Errorf("runs: recovered tree %q has no terminal root", rootRunID)
	}
	record := goal.RunRecord{
		SessionID:     lostRoot.SessionID(),
		IncarnationID: lostRoot.GoalIncarnationID(),
		RunID:         lostRoot.ID(),
		Outcome:       outcome,
		Steps:         lostRoot.Metrics().Steps(),
		CompletedAt:   lostRoot.FinishedAt(),
	}
	if usage, reported := lostRoot.Metrics().Usage(); reported && usage.Total.CostUSD != nil {
		record.CostUSD = *usage.Total.CostUSD
	}
	return record, nil
}

type recoveryRunTree struct {
	root      rundomain.Run
	runsByID  map[string]rundomain.Run
	postorder []string
}

func groupRecoveryRunTrees(active []rundomain.Run) (map[string]recoveryRunTree, error) {
	grouped := make(map[string][]rundomain.Run)
	for index, run := range active {
		if err := run.Validate(); err != nil {
			return nil, fmt.Errorf("runs: validate recovery Run[%d] %q: %w", index, run.ID(), err)
		}
		rootRunID := run.Lineage().TreeRootID(run.ID())
		grouped[rootRunID] = append(grouped[rootRunID], run)
	}

	trees := make(map[string]recoveryRunTree, len(grouped))
	for rootRunID, runs := range grouped {
		members := make([]rundomain.TreeMember, 0, len(runs))
		runsByID := make(map[string]rundomain.Run, len(runs))
		for _, run := range runs {
			members = append(members, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
			runsByID[run.ID()] = run
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
			if run.SessionID() != root.SessionID() {
				return nil, fmt.Errorf(
					"runs: recovery Run %q belongs to Session %q, want tree Session %q",
					run.ID(),
					run.SessionID(),
					root.SessionID(),
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
) ([]rundomain.Run, []ItemReplacement, error) {
	lostRuns := make([]rundomain.Run, 0, len(tree.postorder))
	var replacements []ItemReplacement
	for _, runID := range tree.postorder {
		active := tree.runsByID[runID]
		for _, item := range items {
			if item.RunID() != active.ID() || item.Status() != transcript.ItemRunning {
				continue
			}
			failure := tool.Failure{
				Kind:   tool.FailureExecution,
				Detail: "tool call interrupted because the run was lost on restart",
			}
			replacement, err := item.AbandonToolCall(&failure, finishedAt)
			if err != nil {
				return nil, nil, fmt.Errorf("runs: recover lost Item %q: %w", item.ID(), err)
			}
			replacements = append(replacements, ItemReplacement{Expected: item, Replacement: replacement})
		}

		lost, err := active.RecoverLost(rundomain.Failure{
			Kind: rundomain.FailureLost, Detail: "run lost on restart",
		}, finishedAt, messageMark)
		if err != nil {
			return nil, nil, fmt.Errorf("runs: recover lost Run %q: %w", active.ID(), err)
		}
		lostRuns = append(lostRuns, lost)
	}
	return lostRuns, replacements, nil
}
