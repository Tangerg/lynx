package runs

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

var (
	errExecutorContract = errors.New("runs: executor contract violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

// reduction is one canonical output plus the persisted fact and live nudge that
// arise from the same EngineEvent decision. The pump commits it before placing
// Event on the Journal.
type reduction struct {
	Event  RunEvent
	Commit *EventCommit
	Nudge  *Nudge
}

// reductionBatch is the complete publication unit for one executor event. A
// normal batch commits individual event projections in order. A park batch owns
// one explicit write-set that must commit before any of its events become
// visible; keeping that boundary on the batch avoids encoding it as a boolean
// or a privileged first element in the event slice.
type reductionBatch struct {
	events     []reduction
	parkCommit *EventCommit
}

type reducerConfig struct {
	RunID          string
	SegmentID      string
	SessionID      string
	Lineage        execution.RunLineage
	Cwd            string
	TurnID         string
	GoalLeaseID    string
	ModelSelection modelref.Selection
	CreatedAt      time.Time
	UserInput      []transcript.ContentBlock
	// Metrics is what the Run had already consumed before this segment opened —
	// zero for a first segment, the parked Run's accrual for a continuation. Every
	// Run record this reducer commits is the sum of this and the current segment,
	// so a resumed Run reports the Run rather than its latest continuation.
	Metrics transcript.RunMetrics
	// Limits is the allowance in force for the whole Run, frozen at admission and
	// carried unchanged through every continuation.
	Limits execution.RunLimits
	// Capabilities is the Run's frozen optional behavior. Every record this reducer
	// commits carries the admission value, including continuation records.
	Capabilities execution.RunCapabilities
	Continuation *treeContinuation
	Now          func() time.Time
	CancelReason func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical RunEvent family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg     reducerConfig
	resume  *resumeBinding
	itemSeq int
	// step is the latest cumulative accounted model-call count reported by the
	// executor. It uses the same unit as RunLimits.MaxSteps; tool events never
	// infer it.
	step      int
	toolOrder int
	// usage is the latest authoritative cumulative Run accounting reported by
	// the executor. Nil means this segment has not advanced the committed
	// snapshot in cfg.Metrics.
	usage           *transcript.Usage
	segmentDuration time.Duration
	userInput       []transcript.ContentBlock
	text            *openText
	reasoning       *openText
	tools           openTools
	drained         []interrupts.DrainedTool
	errProblem      *transcript.Problem
	// plan is the last state snapshot this segment published, kept so the segment
	// can fence its final value before finishing. Nil means this segment never
	// changed the projection, and a segment that changed nothing has nothing to
	// fence.
	plan *StateSnapshot
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	callID       string
	sourceCallID string
	order        int
	id           string
	startedAt    time.Time
	finishedAt   time.Time
	name         string
	arguments    tool.Arguments
	safetyClass  tool.SafetyClass
	end          *ToolCallEnd
}

func newReducer(cfg reducerConfig) *reducer {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cfg.Now = now
	// The reducer outlives the Start request and publishes UserInput through the
	// journal after admission. Own the slice before it becomes persisted/live
	// state so a caller reusing its command buffer cannot rewrite emitted facts.
	cfg.UserInput = slices.Clone(cfg.UserInput)
	var resume *resumeBinding
	if cfg.Continuation != nil {
		resume = resumeBindingFrom(*cfg.Continuation, cfg.RunID)
	}
	return &reducer{
		cfg: cfg, resume: resume, userInput: transcript.CloneContent(cfg.UserInput), step: cfg.Metrics.Steps,
		tools: make(openTools),
	}
}

func (r *reducer) nextItemID() string {
	r.itemSeq++
	return itemIDPrefix + r.cfg.SegmentID + "_" + strconv.Itoa(r.itemSeq)
}

func userMessageItemID(segmentID string) string { return itemIDPrefix + segmentID + "_u" }

func (r *reducer) open() (reductionBatch, error) {
	if r.resume != nil && r.resume.err != nil {
		return reductionBatch{}, fmt.Errorf("%w: %w", errReducerInvariant, r.resume.err)
	}
	createdAt := r.cfg.CreatedAt
	if createdAt.IsZero() {
		createdAt = r.now()
	}
	// The opening Run record goes through runRecord like every other one, so a
	// resumed segment announces the Run's accrual and allowance rather than a fresh
	// Run's zeros. Only the creation stamp differs: an opening may have to mint one.
	opening := r.runRecord(execution.Running)
	opening.CreatedAt = createdAt
	out := []RunEvent{SegmentStarted{Run: opening}}
	out = append(out, r.openUserMessage()...)
	out = append(out, r.resumeQuestionCompletions()...)
	return r.project(out)
}

func (r *reducer) reduce(ev EngineEvent) (reductionBatch, error) {
	var out []RunEvent
	switch e := ev.(type) {
	case MessageDelta:
		out = r.closeReasoning()
		out = append(out, r.appendText(e.Text)...)
	case ReasoningDelta:
		out = r.closeText()
		out = append(out, r.appendReasoning(e.Text)...)
	case ToolCallStart:
		var err error
		out, err = r.toolStart(e)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: tool call start: %w", errExecutorContract, err)
		}
	case ToolCallEnd:
		var err error
		out, err = r.toolEnd(e)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: tool call end: %w", errExecutorContract, err)
		}
	case UsageReported:
		var err error
		out, err = r.usageProgress(e)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: usage report: %w", errExecutorContract, err)
		}
	case SteerMessage:
		out = r.steerMessage(e)
	case PlanUpdated:
		out = r.planSnapshot(e)
	case CompactBoundary:
		out = r.compaction(e)
	case TurnInterrupted:
		var err error
		out, err = r.interrupt(e)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: interrupt: %w", errExecutorContract, err)
		}
	case TurnEnd:
		var err error
		out, err = r.turnEnd(e)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: turn end: %w", errExecutorContract, err)
		}
	default:
		return reductionBatch{}, fmt.Errorf("%w: unhandled event %T", errExecutorContract, ev)
	}
	return r.project(out)
}

func (r *reducer) synthesizeTerminal() (reductionBatch, error) {
	out := r.closeStreaming()
	drained, err := r.drainTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: drain tools: %w", errReducerInvariant, err)
	}
	out = append(out, drained...)
	// No TurnEnd arrived, so nothing fresh was reported: the segment's accrual
	// stands as last reported and is committed as-is.
	var failure *transcript.Problem
	outcome := execution.OutcomeCanceled
	if r.errProblem != nil {
		outcome = execution.OutcomeError
		failure, err = runResultProblem(*r.errProblem)
		if err != nil {
			return reductionBatch{}, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
		}
	}
	detail := ""
	if outcome == execution.OutcomeCanceled && r.cfg.CancelReason != nil {
		detail = r.cfg.CancelReason()
	}
	terminal, err := r.finishedRun(outcome, failure, detail)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
	}
	out = append(out, terminal)
	return r.project(out)
}

// abort marks the segment as failed so terminal synthesis produces an error
// outcome. It takes no cause: an internal failure must not put its internals on
// the wire, so the client gets the bare symbol and supplies its own words.
// That makes the caller's span the only place the cause survives — a rejected
// terminal commit or a protocol-violating executor event is otherwise invisible
// — so every caller records it there before calling this.
func (r *reducer) abort() {
	r.errProblem = &transcript.Problem{Kind: transcript.InternalProblem, Scope: transcript.RunProblem}
}

// runResultProblem checks that a problem belongs in a run's result slot rather
// than forcing it to fit. Overwriting the scope would silently relabel a
// tool-scoped problem as run-scoped, and the export-time ValidateFor that would
// eventually notice can no longer say which segment produced it.
func runResultProblem(problem transcript.Problem) (*transcript.Problem, error) {
	if err := problem.ValidateFor(transcript.RunProblem); err != nil {
		return nil, fmt.Errorf("run result problem: %w", err)
	}
	return &problem, nil
}

func (r *reducer) project(events []RunEvent) (reductionBatch, error) {
	events = r.fenceFinalState(events)
	out := make([]reduction, 0, len(events))
	for _, event := range events {
		reduced, err := r.projectOne(event)
		if err != nil {
			return reductionBatch{}, err
		}
		out = append(out, reduced)
	}

	// A park is one persistence boundary: any drained/closed items, its running
	// approval/question items, open interrupt record, interrupted transcript run,
	// and admission transition must commit together before ANY event in this
	// batch is published. Build an explicit batch-owned write-set instead of
	// moving it onto a privileged event position.
	parkAt := -1
	for i := range out {
		if out[i].Commit != nil && out[i].Commit.State == StateSuspend {
			if parkAt >= 0 {
				return reductionBatch{}, fmt.Errorf("%w: reduction batch has multiple park boundaries", errReducerInvariant)
			}
			parkAt = i
		}
		if itemStarted, ok := out[i].Event.(ItemStarted); ok {
			itemStarted.Item.SessionID = r.cfg.SessionID
			out[i].Event = itemStarted
		}
	}
	if parkAt >= 0 {
		commit := out[parkAt].Commit
		if commit == nil {
			return reductionBatch{}, fmt.Errorf("%w: park boundary has no projection commit", errReducerInvariant)
		}
		items := make([]transcript.Item, 0, len(out))
		for i, reduced := range out {
			if i != parkAt && reduced.Commit != nil {
				if reduced.Commit.Run != nil || reduced.Commit.State != StateUnchanged {
					return reductionBatch{}, fmt.Errorf("%w: park batch contains another lifecycle transition", errReducerInvariant)
				}
				items = append(items, reduced.Commit.Items...)
			}
			if itemStarted, ok := reduced.Event.(ItemStarted); ok {
				items = append(items, itemStarted.Item)
			}
			out[i].Commit = nil
		}
		commit.Items = items
		batch := reductionBatch{events: out, parkCommit: commit}
		if err := validateReductionBatch(batch); err != nil {
			return reductionBatch{}, err
		}
		return batch, nil
	}
	batch := reductionBatch{events: out}
	if err := validateReductionBatch(batch); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

// fenceFinalState republishes the segment's last state snapshot immediately before
// the segment finishes, for every key the segment changed.
//
// Without it, a client only holds the state if it received the change event itself.
// A subscriber that attached later — or replayed from a cursor past that event —
// reaches segment.finished having never seen a snapshot, and renders a stale panel
// until something makes it refetch. The fence makes the guarantee positional:
// whoever receives the finish has received the final value, because it is the
// replayable event immediately before it.
//
// The repeat is the point, not waste: a latest-value projection carries its own
// revision, so folding it twice is folding it once.
//
// It belongs to the batch rather than to either finish path: a park and a terminal
// are two reasons for one boundary, and a rule stated in both places is a rule that
// drifts in one of them.
func (r *reducer) fenceFinalState(events []RunEvent) []RunEvent {
	if r.plan == nil {
		return events
	}
	for i, event := range events {
		if _, finishing := event.(SegmentFinished); !finishing {
			continue
		}
		fence := *r.plan
		// One fence per segment: a resumed segment fences again only if it changes
		// the projection again.
		r.plan = nil
		return slices.Insert(events, i, RunEvent(fence))
	}
	return events
}

func (r *reducer) projectOne(event RunEvent) (reduction, error) {
	commit := EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		e.Item.SessionID = r.cfg.SessionID
		event = e
		commit.Items = []transcript.Item{e.Item}
		if e.Item.Status == transcript.ItemCompleted && e.Item.Error == nil && len(e.mutatedPaths) > 0 {
			nudge = &Nudge{Cwd: r.cfg.Cwd, Paths: slices.Clone(e.mutatedPaths)}
		}
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State == execution.Interrupted {
			commit.State = StateSuspend
			return reduction{Event: event, Commit: &commit}, nil
		}
		commit.State = StateTerminalize
		if e.Run.Outcome != nil {
			commit.Outcome = *e.Run.Outcome
			commit.GoalTurn = r.goalTurn(e.Run)
		}
	case SegmentStarted, SegmentProgressed, ItemStarted, ItemChanged, StateSnapshot:
		// These events have no standalone EventCommit. SegmentStarted carries a Run
		// for the stream, but the Run's durable opening IS its admission (or its
		// resume) — recording it a second time here would be a second writer of
		// facts admission already owns. Interrupt ItemStarted projections are folded
		// into the atomic park write-set by project.
	default:
		return reduction{}, fmt.Errorf("%w: unhandled run event %T", errReducerInvariant, event)
	}
	var eventCommit *EventCommit
	if !commit.isEmpty() {
		eventCommit = &commit
	}
	return reduction{Event: event, Commit: eventCommit, Nudge: nudge}, nil
}

func (r *reducer) goalTurn(run transcript.Run) *goal.TurnRecord {
	if r.cfg.GoalLeaseID == "" || run.Outcome == nil {
		return nil
	}
	record := &goal.TurnRecord{
		SessionID:   r.cfg.SessionID,
		LeaseID:     r.cfg.GoalLeaseID,
		RunID:       r.cfg.RunID,
		Outcome:     *run.Outcome,
		CompletedAt: run.FinishedAt,
	}
	if record.CompletedAt.IsZero() {
		record.CompletedAt = r.now()
	}
	record.Steps = run.Metrics.Steps
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		record.CostUSD = *run.Metrics.Usage.CostUSD
	}
	return record
}

// validateReductionBatch checks the pump-facing shape before any commit or
// publication occurs. The reducer normally constructs this shape itself; the
// second check keeps future projection changes from creating partial persistence
// boundaries.
func validateReductionBatch(batch reductionBatch) error {
	if len(batch.events) == 0 {
		if batch.parkCommit != nil {
			return fmt.Errorf("%w: empty batch has a park commit", errReducerInvariant)
		}
		return nil
	}

	terminalAt, err := validateReductionEvents(batch.events)
	if err != nil {
		return err
	}
	if batch.parkCommit != nil {
		return validateParkReductionBatch(batch, terminalAt)
	}
	if terminalAt < 0 {
		return nil
	}
	return validateTerminalReduction(batch.events[terminalAt])
}

// lifecycleReductions counts the events that move the segment's lifecycle: its
// opening and its ending.
func lifecycleReductions(reductions []reduction) int {
	count := 0
	for _, reduced := range reductions {
		switch reduced.Event.(type) {
		case SegmentStarted, SegmentFinished:
			count++
		}
	}
	return count
}

func validateReductionEvents(reductions []reduction) (terminalAt int, err error) {
	terminalAt = -1
	for i, reduced := range reductions {
		if reduced.Event == nil {
			return -1, fmt.Errorf("%w: reduction[%d] has no event", errReducerInvariant, i)
		}
		// One batch moves the segment's lifecycle at most once, and an opening only
		// ever starts one. Both are checked on the events rather than on their
		// projection commits because an opening produces none — a Run's persisted
		// opening is its admission — so a stray one would otherwise pass unseen.
		if _, opening := reduced.Event.(SegmentStarted); opening {
			if i != 0 {
				return -1, fmt.Errorf("%w: reduction[%d] opens a segment mid-batch", errReducerInvariant, i)
			}
			if lifecycleReductions(reductions) > 1 {
				return -1, fmt.Errorf("%w: reduction batch both opens and ends a segment", errReducerInvariant)
			}
		}
		if reduced.Event.Terminal() {
			terminalAt = i
			if i != len(reductions)-1 {
				return -1, fmt.Errorf("%w: terminal reduction[%d] is not last", errReducerInvariant, i)
			}
		}
		if reduced.Commit != nil {
			switch reduced.Commit.State {
			case StateUnchanged:
			case StateSuspend:
				return -1, fmt.Errorf("%w: reduction[%d] carries a park commit", errReducerInvariant, i)
			case StateTerminalize:
				if !reduced.Event.Terminal() {
					return -1, fmt.Errorf("%w: terminal commit at reduction[%d] has no terminal event", errReducerInvariant, i)
				}
			default:
				return -1, fmt.Errorf("%w: reduction[%d] has unknown state change %d", errReducerInvariant, i, reduced.Commit.State)
			}
		}
	}
	return terminalAt, nil
}

func validateParkReductionBatch(batch reductionBatch, terminalAt int) error {
	for i, reduced := range batch.events {
		if reduced.Commit != nil {
			return fmt.Errorf("%w: park batch event[%d] repeats a projection commit", errReducerInvariant, i)
		}
	}
	commit := batch.parkCommit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: park batch has no projection commit", errReducerInvariant)
	case commit.State != StateSuspend:
		return fmt.Errorf("%w: park batch commit does not suspend the run", errReducerInvariant)
	case commit.Run == nil || commit.Run.State != execution.Interrupted:
		return fmt.Errorf("%w: park batch commit has no interrupted run", errReducerInvariant)
	case terminalAt != len(batch.events)-1:
		return fmt.Errorf("%w: park batch has no terminal boundary event", errReducerInvariant)
	}
	return nil
}

func validateTerminalReduction(reduced reduction) error {
	commit := reduced.Commit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: terminal event has no projection commit", errReducerInvariant)
	case commit.State != StateTerminalize:
		return fmt.Errorf("%w: terminal event commit does not terminalize the run", errReducerInvariant)
	case commit.Run == nil || !commit.Run.State.IsTerminal():
		return fmt.Errorf("%w: terminal event commit has no terminal run", errReducerInvariant)
	case commit.Run.Outcome == nil || *commit.Run.Outcome != commit.Outcome:
		return fmt.Errorf("%w: terminal event commit has an inconsistent outcome", errReducerInvariant)
	case commit.GoalTurn != nil && (commit.GoalTurn.RunID != commit.RunID || commit.GoalTurn.SessionID != commit.SessionID || commit.GoalTurn.Outcome != commit.Outcome):
		return fmt.Errorf("%w: terminal event commit has an inconsistent goal turn", errReducerInvariant)
	}
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errReducerInvariant, err)
	}
	wantState, ok := execution.Running.Terminate(commit.Outcome)
	if !ok || commit.Run.State != wantState {
		return fmt.Errorf("%w: terminal event commit has an invalid lifecycle transition", errReducerInvariant)
	}
	return nil
}

func (r *reducer) now() time.Time { return r.cfg.Now().UTC() }
