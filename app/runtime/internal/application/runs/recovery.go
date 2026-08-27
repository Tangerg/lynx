package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	rundomain "github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
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

// RecoveryAdmissions provides the same per-Session writer boundary used by
// live Run and Session mutations. Recovery skips a Session when another
// Runtime process still owns it; such a Run is live, not abandoned.
type RecoveryAdmissions interface {
	AcquireSession(sessionID string) (release func(), ok bool)
}

// RecoveryCommit is the complete atomic write-set for one ownership-scoped
// reconciliation. LostRuns are ordered child-before-parent. Checkpoint and
// callback cleanup names only Sessions whose writer lease was acquired.
type RecoveryCommit struct {
	LostRuns                   []rundomain.Run
	ItemReplacements           []ItemReplacement
	ConversationTransitions    []RecoveryConversationTransition
	ModelInvocations           []ModelInvocationRecovery
	ToolInvocations            []ToolInvocationRecovery
	GoalRuns                   []goal.RunRecord
	DeleteInterrupts           []InterruptOwner
	PreservedCheckpointRootIDs []string
	// RecoveredSessionIDs is the exact set whose abandoned callback ledger
	// may be retired. It never names a Session whose writer lease was contended.
	RecoveredSessionIDs []string
	// DeleteCheckpointSessionIDs names recovered lost trees. A preserved waiting
	// tree is intentionally absent so its opaque checkpoint remains available.
	DeleteCheckpointSessionIDs []string
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
// a dead Runtime process. Each pass claims Session writer ownership before
// re-reading candidates; CommitRecovery applies the resulting write-set
// atomically and Run transitions remain CAS guarded by storage.
type Recovery struct {
	store         RecoveryStore
	resumability  WaitingExecutionResumability
	admissions    RecoveryAdmissions
	invalidations invalidation.Publish
	now           func() time.Time
}

// recoveryPlanner owns one ownership-scoped reconciliation snapshot and the caches needed
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

// NewRecovery constructs the Run ownership recovery use case.
func NewRecovery(
	store RecoveryStore,
	resumability WaitingExecutionResumability,
	admissions RecoveryAdmissions,
	invalidations invalidation.Publish,
) (*Recovery, error) {
	if store == nil {
		return nil, errors.New("runs: recovery store is required")
	}
	if resumability == nil {
		return nil, errors.New("runs: waiting execution resumability is required")
	}
	if admissions == nil {
		return nil, errors.New("runs: recovery admissions are required")
	}
	return &Recovery{
		store:         store,
		resumability:  resumability,
		admissions:    admissions,
		invalidations: invalidations,
		now:           time.Now,
	}, nil
}

// Reconcile preserves only complete waiting trees whose durable hand-off
// and opaque executor checkpoint remain coherent. Every other non-terminal tree
// is recovered as run_lost in one application transaction.
func (r *Recovery) Reconcile(ctx context.Context) (int, error) {
	claims, err := r.claimAbandonedSessions(ctx)
	if err != nil {
		return 0, err
	}
	defer claims.release()
	planner, err := newRecoveryPlanner(ctx, r, claims)
	if err != nil {
		return 0, err
	}
	commit, reconciled, err := planner.plan()
	if err != nil {
		return 0, err
	}
	if err := r.store.CommitRecovery(ctx, commit); err != nil {
		return 0, fmt.Errorf("runs: commit ownership recovery: %w", err)
	}
	r.publishRecoveredReadModels(commit)
	return reconciled, nil
}

// recoverySessionClaims owns the Session writer claims for one reconciliation
// pass. The claimed snapshot and release stack cannot be separated, and release
// is exact-once across every planning or commit failure.
type recoverySessionClaims struct {
	active     []rundomain.Run
	sessionIDs map[string]struct{}
	releases   []func()
	released   bool
}

func (r *recoverySessionClaims) includes(sessionID string) bool {
	_, ok := r.sessionIDs[sessionID]
	return ok
}

func (r *recoverySessionClaims) release() {
	if r == nil || r.released {
		return
	}
	r.released = true
	for index := len(r.releases) - 1; index >= 0; index-- {
		r.releases[index]()
	}
}

// publishRecoveredReadModels reports only the durable fact scopes this recovery
// write-set can change. Periodic survivor recovery commits through this Runtime's
// own SQLite connection, so PRAGMA data_version deliberately cannot observe it;
// without an application notice, already-mounted clients would remain pinned to
// the dead owner's Run/HITL projection even after the database reached RunLost.
func (r *Recovery) publishRecoveredReadModels(commit RecoveryCommit) {
	type sessionChanges struct {
		runIDs  []string
		rootIDs []string
		goal    bool
	}
	changes := make(map[string]*sessionChanges)
	for _, lost := range commit.LostRuns {
		scope := changes[lost.SessionID()]
		if scope == nil {
			scope = &sessionChanges{}
			changes[lost.SessionID()] = scope
		}
		scope.runIDs = append(scope.runIDs, lost.ID())
	}
	if len(changes) == 0 {
		return
	}
	for _, owner := range commit.DeleteInterrupts {
		if scope := changes[owner.SessionID]; scope != nil {
			scope.rootIDs = append(scope.rootIDs, owner.RootRunID)
		}
	}
	for _, record := range commit.GoalRuns {
		if scope := changes[record.SessionID]; scope != nil {
			scope.goal = true
		}
	}
	sessionIDs := make([]string, 0, len(changes))
	for sessionID := range changes {
		sessionIDs = append(sessionIDs, sessionID)
	}
	slices.Sort(sessionIDs)
	for _, sessionID := range sessionIDs {
		scope := changes[sessionID]
		slices.Sort(scope.runIDs)
		scope.runIDs = slices.Compact(scope.runIDs)
		slices.Sort(scope.rootIDs)
		scope.rootIDs = slices.Compact(scope.rootIDs)
		notices := []invalidation.Notice{
			invalidation.InSession(invalidation.Runs, sessionID, scope.runIDs...),
		}
		if len(scope.rootIDs) > 0 {
			notices = append(notices,
				invalidation.InSession(invalidation.Interrupts, sessionID, scope.rootIDs...),
			)
		}
		notices = append(notices, invalidation.InSession(invalidation.Sessions, sessionID))
		if scope.goal {
			notices = append(notices, invalidation.InSession(invalidation.Goals, sessionID))
		}
		r.invalidations.Notify(notices...)
	}
}

func (r *Recovery) claimAbandonedSessions(
	ctx context.Context,
) (*recoverySessionClaims, error) {
	candidates, err := r.store.ListNonTerminalRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("runs: load recovery candidates: %w", err)
	}
	ids := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.SessionID()]; ok {
			continue
		}
		seen[candidate.SessionID()] = struct{}{}
		ids = append(ids, candidate.SessionID())
	}
	slices.Sort(ids)
	claims := &recoverySessionClaims{
		sessionIDs: make(map[string]struct{}, len(ids)),
		releases:   make([]func(), 0, len(ids)),
	}
	for _, sessionID := range ids {
		if release, ok := r.admissions.AcquireSession(sessionID); ok {
			claims.sessionIDs[sessionID] = struct{}{}
			claims.releases = append(claims.releases, release)
		}
	}
	current, err := r.store.ListNonTerminalRuns(ctx)
	if err != nil {
		claims.release()
		return nil, fmt.Errorf("runs: reload claimed recovery Runs: %w", err)
	}
	claims.active = make([]rundomain.Run, 0, len(current))
	for _, candidate := range current {
		if claims.includes(candidate.SessionID()) {
			claims.active = append(claims.active, candidate)
		}
	}
	return claims, nil
}

func newRecoveryPlanner(
	ctx context.Context,
	recovery *Recovery,
	claims *recoverySessionClaims,
) (*recoveryPlanner, error) {
	if recovery == nil {
		return nil, errors.New("runs: recovery use case is required")
	}
	if claims == nil {
		return nil, errors.New("runs: recovery Session claims are required")
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
	activeRoots := make(map[string]struct{}, len(claims.active))
	for _, value := range claims.active {
		activeRoots[value.Lineage().TreeRootID(value.ID())] = struct{}{}
	}
	pending = slices.DeleteFunc(pending, func(open Pending) bool {
		_, ok := activeRoots[open.RootRunID]
		return !ok
	})
	modelInvocations = slices.DeleteFunc(modelInvocations, func(open OpenModelInvocation) bool {
		return !claims.includes(open.SessionID)
	})
	toolInvocations = slices.DeleteFunc(toolInvocations, func(open OpenToolInvocation) bool {
		return !claims.includes(open.SessionID)
	})
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

	trees, err := groupRecoveryRunTrees(claims.active)
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
		finishedAt:    recovery.now().UTC(),
	}
	for sessionID := range claims.sessionIDs {
		planner.commit.RecoveredSessionIDs = append(planner.commit.RecoveredSessionIDs, sessionID)
	}
	slices.Sort(planner.commit.RecoveredSessionIDs)
	// Recovery is a new durable observation in the same timelines as every
	// fact it closes. Wall time can move backward across a reboot, so derive the
	// observation timestamp from the complete boot snapshot instead of allowing
	// a regressed clock to make otherwise-recoverable state permanently invalid.
	for _, active := range claims.active {
		planner.observeTime(active.UpdatedAt())
	}
	for _, open := range pending {
		planner.observeTime(open.CreatedAt)
	}
	for _, invocation := range modelInvocations {
		planner.observeTime(invocation.StartedAt)
	}
	for _, invocation := range toolInvocations {
		planner.observeTime(invocation.StartedAt)
	}
	// Every active tree's Transcript is required by planTree anyway. Preload it
	// before constructing any terminal facts so a later Item in one tree cannot
	// advance the shared recovery timestamp after another tree was planned.
	for _, tree := range trees {
		if _, err := planner.transcript(tree.root.SessionID()); err != nil {
			return nil, err
		}
	}
	for _, invocation := range modelInvocations {
		planner.commit.ModelInvocations = append(planner.commit.ModelInvocations, ModelInvocationRecovery{
			SessionID: invocation.SessionID, RunID: invocation.RunID, SegmentID: invocation.SegmentID,
			CallID: invocation.CallID, StartedAt: invocation.StartedAt, FinishedAt: planner.finishedAt,
		})
	}
	for _, invocation := range toolInvocations {
		planner.commit.ToolInvocations = append(planner.commit.ToolInvocations, ToolInvocationRecovery{
			SessionID: invocation.SessionID, RunID: invocation.RunID, SegmentID: invocation.SegmentID,
			CallID: invocation.CallID, ItemID: invocation.ItemID,
			StartedAt: invocation.StartedAt, FinishedAt: planner.finishedAt,
		})
	}
	return planner, nil
}

func (r *recoveryPlanner) observeTime(value time.Time) {
	value = value.UTC()
	if value.After(r.finishedAt) {
		r.finishedAt = value
	}
}

func (r *recoveryPlanner) plan() (RecoveryCommit, int, error) {
	rootRunIDs := make([]string, 0, len(r.trees))
	for rootRunID := range r.trees {
		rootRunIDs = append(rootRunIDs, rootRunID)
	}
	slices.Sort(rootRunIDs)
	for _, rootRunID := range rootRunIDs {
		if err := r.planTree(rootRunID); err != nil {
			return RecoveryCommit{}, 0, err
		}
	}
	for _, open := range r.pending {
		if _, preserved := r.preserved[open.RootRunID]; preserved {
			root, _ := open.RootContinuation()
			r.commit.PreservedCheckpointRootIDs = append(
				r.commit.PreservedCheckpointRootIDs,
				root.MemberID,
			)
		}
	}
	slices.SortFunc(r.commit.DeleteInterrupts, func(left, right InterruptOwner) int {
		if bySession := strings.Compare(left.SessionID, right.SessionID); bySession != 0 {
			return bySession
		}
		return strings.Compare(left.RootRunID, right.RootRunID)
	})
	slices.SortFunc(r.commit.ModelInvocations, compareModelInvocationRecoveries)
	slices.SortFunc(r.commit.ToolInvocations, compareToolInvocationRecoveries)
	slices.Sort(r.commit.PreservedCheckpointRootIDs)
	slices.Sort(r.commit.DeleteCheckpointSessionIDs)
	if err := r.commit.Validate(); err != nil {
		return RecoveryCommit{}, 0, err
	}
	return r.commit, r.reconciled, nil
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

func (r *recoveryPlanner) planTree(rootRunID string) error {
	tree := r.trees[rootRunID]
	items, err := r.transcript(tree.root.SessionID())
	if err != nil {
		return err
	}
	open, hasInterrupt := r.pendingByRoot[rootRunID]
	if tree.root.State() == rundomain.Waiting && hasInterrupt {
		sess, sessionErr := r.session(tree.root.SessionID())
		if sessionErr != nil {
			return sessionErr
		}
		resumable, sessionErr := validateRecoveryParkedTree(
			r.ctx,
			tree,
			open,
			sess,
			items,
			r.store,
			r.resumability,
		)
		if sessionErr != nil {
			return sessionErr
		}
		if resumable {
			r.preserved[rootRunID] = struct{}{}
			return nil
		}
	}
	conversationSnapshot, err := r.conversation(tree.root.SessionID())
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
	lostRuns, replacements, err := recoverLostTree(tree, items, messageMark, r.finishedAt)
	if err != nil {
		return err
	}
	r.commit.LostRuns = append(r.commit.LostRuns, lostRuns...)
	r.commit.ItemReplacements = append(r.commit.ItemReplacements, replacements...)
	r.commit.ConversationTransitions = append(
		r.commit.ConversationTransitions,
		RecoveryConversationTransition{
			RootRunID: tree.root.ID(), SessionID: tree.root.SessionID(),
			ExpectedCount: conversationSnapshot.count, Messages: closure,
		},
	)
	r.commit.DeleteInterrupts = append(r.commit.DeleteInterrupts, InterruptOwner{
		SessionID: tree.root.SessionID(),
		RootRunID: tree.root.ID(),
	})
	r.commit.DeleteCheckpointSessionIDs = append(
		r.commit.DeleteCheckpointSessionIDs,
		tree.root.SessionID(),
	)
	if tree.root.GoalIncarnationID() != "" {
		record, err := recoveredGoalRun(tree.root.ID(), lostRuns)
		if err != nil {
			return err
		}
		r.commit.GoalRuns = append(r.commit.GoalRuns, record)
	}
	r.reconciled += len(lostRuns)
	return nil
}

func (r *recoveryPlanner) transcript(sessionID string) ([]transcript.Item, error) {
	if items, ok := r.transcripts[sessionID]; ok {
		return items, nil
	}
	items, err := r.store.ListTranscript(r.ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("runs: load recovery transcript for Session %q: %w", sessionID, err)
	}
	for _, item := range items {
		r.observeTime(item.OccurredAt())
		r.observeTime(item.FinishedAt())
	}
	r.transcripts[sessionID] = items
	return items, nil
}

func (r *recoveryPlanner) session(sessionID string) (session.Session, error) {
	if sess, ok := r.sessions[sessionID]; ok {
		return sess, nil
	}
	sess, err := r.store.SessionByID(r.ctx, sessionID)
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
	r.sessions[sessionID] = sess
	return sess, nil
}

func (r *recoveryPlanner) conversation(sessionID string) (recoveryConversationSnapshot, error) {
	if snapshot, ok := r.conversations[sessionID]; ok {
		return snapshot, nil
	}
	messages, err := r.store.ReadMessages(r.ctx, sessionID)
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
	count, err := r.store.CountMessages(r.ctx, sessionID)
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
	r.conversations[sessionID] = snapshot
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
