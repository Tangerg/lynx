package agentexec

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/core/chat"
)

type notifyingRunningSubtreeCanceler struct {
	inner    runs.RunningSubtreeCanceler
	accepted func()
}

func (canceler notifyingRunningSubtreeCanceler) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	reason string,
) error {
	if err := canceler.inner.CancelRunningSubtree(ctx, ref, memberID, reason); err != nil {
		return err
	}
	if canceler.accepted != nil {
		canceler.accepted()
	}
	return nil
}

type cancelableDelegateModel struct {
	defaults *chat.Options

	childStartedOnce        sync.Once
	rootStartedOnce         sync.Once
	childCallStarted        chan struct{}
	childCallReturned       chan struct{}
	rootContinuationStarted chan struct{}
	releaseRootContinuation chan struct{}
}

func newCancelableDelegateModel() *cancelableDelegateModel {
	return &cancelableDelegateModel{
		defaults:                &chat.Options{Model: "stub-cancelable-delegate"},
		childCallStarted:        make(chan struct{}),
		childCallReturned:       make(chan struct{}),
		rootContinuationStarted: make(chan struct{}),
		releaseRootContinuation: make(chan struct{}),
	}
}

func (model *cancelableDelegateModel) DefaultOptions() chat.Options { return *model.defaults }

func (model *cancelableDelegateModel) Call(
	ctx context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	switch {
	case toolResult(request.Messages, "delegate_task") != "":
		model.rootStartedOnce.Do(func() { close(model.rootContinuationStarted) })
		select {
		case <-model.releaseRootContinuation:
			return interactionUsageTextResponse("root completed after child cancellation", 2, 1), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case userMessagesContain(request.Messages, "child waits for cancellation"):
		model.childStartedOnce.Do(func() { close(model.childCallStarted) })
		<-ctx.Done()
		close(model.childCallReturned)
		return nil, ctx.Err()
	case userMessagesContain(request.Messages, "delegate cancelable work"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_cancelable", Name: "delegate_task",
			Arguments: `{"summary":"cancelable child","instructions":"child waits for cancellation"}`,
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected cancelable Delegate model context")
	}
}

func (model *cancelableDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	response, err := model.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

type waitingDelegateModel struct {
	mu       sync.Mutex
	calls    int
	defaults *chat.Options
}

func newWaitingDelegateModel() *waitingDelegateModel {
	return &waitingDelegateModel{defaults: &chat.Options{Model: "stub-waiting-delegate"}}
}

func (model *waitingDelegateModel) DefaultOptions() chat.Options { return *model.defaults }

func (model *waitingDelegateModel) Call(
	_ context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	model.mu.Lock()
	model.calls++
	model.mu.Unlock()
	switch {
	case toolResult(request.Messages, "ask") != "":
		return interactionUsageTextResponse("child: restored value accepted", 2, 1), nil
	case toolResult(request.Messages, "delegate_task") != "":
		return interactionUsageTextResponse("root: delegated work complete", 2, 1), nil
	case userMessagesContain(request.Messages, "child needs input"):
		return interactionToolResponse(chat.ToolCall{ID: "ask_child", Name: "ask", Arguments: `{}`}, 2, 1), nil
	case userMessagesContain(request.Messages, "delegate waiting work"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_waiting", Name: "delegate_task",
			Arguments: `{"summary":"waiting child","instructions":"child needs input"}`,
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected waiting Delegate model context")
	}
}

func (model *waitingDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	response, err := model.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func (model *waitingDelegateModel) Calls() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}

type delegateConversation struct{}

func (delegateConversation) Read(context.Context, string) ([]chat.Message, error) {
	return nil, nil
}

type delegateSessionStore struct{ value session.Session }

func (store *delegateSessionStore) Get(_ context.Context, id string) (session.Session, error) {
	if id != store.value.ID() {
		return session.Session{}, errors.New("session not found")
	}
	return store.value, nil
}

func (store *delegateSessionStore) Create(
	context.Context,
	string,
	string,
) (session.Session, error) {
	return store.value, nil
}

func (store *delegateSessionStore) PrepareScheduled(
	context.Context,
	string,
	string,
	string,
	string,
) (session.Session, *session.Session, error) {
	return store.value, nil, nil
}

func (*delegateSessionStore) ActiveRun(
	context.Context,
	string,
) (run.Run, bool, error) {
	return run.Run{}, false, nil
}

func (*delegateSessionStore) ListOpenInterrupts(
	context.Context,
	string,
) ([]runs.Pending, error) {
	return nil, nil
}

func (*delegateSessionStore) LookupOpenInterrupt(
	context.Context,
	string,
) (runs.Pending, bool, error) {
	return runs.Pending{}, false, nil
}

func (*delegateSessionStore) ApplyRunCancel(
	context.Context,
	string,
	string,
	string,
	time.Time,
) (run.Run, error) {
	return run.Run{}, errors.New("unexpected parked Run cancellation")
}

func (*delegateSessionStore) ApplyRunLost(
	context.Context,
	string,
	string,
	time.Time,
) error {
	return errors.New("unexpected Run loss")
}

func (*delegateSessionStore) ApplyClaimedRunLost(
	context.Context,
	runs.Pending,
	time.Time,
) error {
	return errors.New("unexpected claimed Resume loss")
}

type delegateProjection struct {
	mu           sync.Mutex
	openings     []runs.OpeningCommit
	barriers     []runs.TreeBarrierCommit
	reservations map[string]runs.ChildRunStartReservation
	outcomes     map[string]runs.ChildRunStartOutcome
	runs         map[string]run.Run
	items        map[string]transcript.Item
}

func newDelegateProjection() *delegateProjection {
	return &delegateProjection{
		reservations: make(map[string]runs.ChildRunStartReservation),
		outcomes:     make(map[string]runs.ChildRunStartOutcome),
		runs:         make(map[string]run.Run),
		items:        make(map[string]transcript.Item),
	}
}

func (projection *delegateProjection) CommitOpening(
	_ context.Context,
	opening runs.OpeningCommit,
) error {
	if err := opening.Validate(); err != nil {
		return err
	}
	projection.mu.Lock()
	projection.applyOpening(opening)
	projection.openings = append(projection.openings, opening)
	projection.mu.Unlock()
	return nil
}

func (projection *delegateProjection) ReserveChildRunStart(
	_ context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	if err := reservation.Validate(); err != nil {
		return err
	}
	projection.mu.Lock()
	defer projection.mu.Unlock()
	memberID := reservation.Member.MemberID
	if existing, found := projection.reservations[memberID]; found && existing != reservation {
		return errors.New("child reservation conflict")
	}
	projection.reservations[memberID] = reservation
	return nil
}

func (projection *delegateProjection) CommitStartedChildRun(
	_ context.Context,
	reservation runs.ChildRunStartReservation,
	opening runs.OpeningCommit,
) error {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	memberID := reservation.Member.MemberID
	if projection.reservations[memberID] != reservation {
		return errors.New("started child has no reservation")
	}
	if prior := projection.outcomes[memberID]; prior != 0 {
		if prior != runs.ChildRunStarted {
			return errors.New("child outcome conflict")
		}
		return nil
	}
	projection.outcomes[memberID] = runs.ChildRunStarted
	projection.applyOpening(opening)
	projection.openings = append(projection.openings, opening)
	return nil
}

func (projection *delegateProjection) AbortChildRunStart(
	_ context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	memberID := reservation.Member.MemberID
	if projection.reservations[memberID] != reservation {
		return errors.New("aborted child has no reservation")
	}
	if prior := projection.outcomes[memberID]; prior != 0 && prior != runs.ChildRunStartAborted {
		return errors.New("child outcome conflict")
	}
	projection.outcomes[memberID] = runs.ChildRunStartAborted
	return nil
}

func (projection *delegateProjection) CommitEvent(
	_ context.Context,
	commit runs.EventCommit,
) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	projection.mu.Lock()
	projection.applyCommit(commit)
	projection.mu.Unlock()
	return nil
}

func (projection *delegateProjection) CommitTreeBarrier(
	_ context.Context,
	barrier runs.TreeBarrierCommit,
) error {
	if err := barrier.Validate(); err != nil {
		return err
	}
	projection.mu.Lock()
	for _, commit := range barrier.Runs {
		projection.applyCommit(commit)
	}
	projection.barriers = append(projection.barriers, barrier)
	projection.mu.Unlock()
	return nil
}

func (projection *delegateProjection) ReadWaitingCheckpoint(
	_ context.Context,
	rootMemberID string,
) (runs.ExecutorCheckpoint, error) {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	for index := len(projection.barriers) - 1; index >= 0; index-- {
		checkpoint := projection.barriers[index].Checkpoint
		if checkpoint.RootMemberID == rootMemberID {
			return checkpoint.Clone(), nil
		}
	}
	return runs.ExecutorCheckpoint{}, runs.ErrExecutorCheckpointNotFound
}

func (*delegateProjection) Nudge(string, []string) {}

func (*delegateProjection) Finish(context.Context, runs.Finish) error { return nil }

func (projection *delegateProjection) Run(
	_ context.Context,
	runID string,
) (run.Run, bool, error) {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	value, found := projection.runs[runID]
	return value, found, nil
}

func (projection *delegateProjection) Tree(
	_ context.Context,
	runID string,
) ([]run.Run, error) {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	target, found := projection.runs[runID]
	if !found {
		return nil, nil
	}
	rootRunID := target.Lineage().TreeRootID(target.ID())
	result := make([]run.Run, 0, len(projection.runs))
	for _, candidate := range projection.runs {
		if candidate.Lineage().TreeRootID(candidate.ID()) == rootRunID {
			result = append(result, candidate)
		}
	}
	slices.SortFunc(result, func(left, right run.Run) int {
		return strings.Compare(left.ID(), right.ID())
	})
	return result, nil
}

func (projection *delegateProjection) Item(
	_ context.Context,
	itemID string,
) (transcript.Item, bool, error) {
	projection.mu.Lock()
	defer projection.mu.Unlock()
	value, found := projection.items[itemID]
	return value, found, nil
}

func (projection *delegateProjection) applyOpening(opening runs.OpeningCommit) {
	if opening.Admit != nil {
		draft := opening.Admit
		projection.runs[draft.RunID] = runfixture.MustRestore(run.Snapshot{ID: draft.RunID, SessionID: draft.SessionID,

			State: run.Running, ActiveSegmentID: draft.SegmentID,
			ModelSelection: draft.ModelSelection, GoalIncarnationID: draft.GoalIncarnationID,
			Limits: draft.Limits, Capabilities: draft.Capabilities,
			CreatedAt: draft.CreatedAt, UpdatedAt: draft.CreatedAt,
			MessageMark: run.UnknownMessageMark, Lineage: run.Lineage{SpawnedByItemID: draft.SpawnedByItemID, ParentRunID: draft.ParentRunID,
				RootRunID: draft.RootRunID}})

	}
	for _, commit := range opening.Events {
		projection.applyCommit(commit)
	}
}

func (projection *delegateProjection) applyCommit(commit runs.EventCommit) {
	for _, item := range commit.Items {
		projection.items[item.ID()] = item
	}
	if commit.Run != nil {
		projection.runs[commit.Run.ID()] = *commit.Run
		return
	}
	if commit.Progress != nil {
		value, found := projection.runs[commit.RunID]
		if found {
			advanced, err := value.AdvanceMetrics(commit.Progress.Metrics, commit.Progress.UpdatedAt)
			if err != nil {
				panic(err)
			}
			projection.runs[commit.RunID] = advanced
		}
	}
}
