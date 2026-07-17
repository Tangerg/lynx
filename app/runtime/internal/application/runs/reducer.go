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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

var (
	errExecutorProtocol = errors.New("runs: executor protocol violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

// reduction is one canonical output plus the durable fact and live nudge that
// arise from the same EngineEvent decision. The pump commits it before placing
// Event on the Journal.
type reduction struct {
	Event     RunEvent
	Commit    *EventCommit
	Nudge     *Nudge
	Interrupt bool
}

type reducerConfig struct {
	RunID        string
	SegmentID    string
	SessionID    string
	Cwd          string
	TurnID       string
	Provider     string
	Model        string
	CreatedAt    time.Time
	UserInput    []ContentBlock
	Pending      *interrupts.Pending
	Now          func() time.Time
	CancelReason func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical RunEvent family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg       reducerConfig
	resume    *resumeBinding
	itemSeq   int
	step      int
	toolOrder int
	userInput []ContentBlock
	text      *openText
	reasoning *openText
	tools     openTools
	drained   []interrupts.DrainedTool
	errMsg    string
	errCode   string
}

type openText struct {
	id        string
	createdAt time.Time
	buf       strings.Builder
}

type openTool struct {
	callID      string
	order       int
	id          string
	createdAt   time.Time
	name        string
	args        string
	safetyClass tool.SafetyClass
	end         *ToolCallEnd
}

func newReducer(cfg reducerConfig) *reducer {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cfg.Now = now
	var resume *resumeBinding
	if cfg.Pending != nil {
		resume = resumeBindingFrom(*cfg.Pending)
	}
	return &reducer{
		cfg: cfg, resume: resume, userInput: cfg.UserInput,
		tools: make(openTools),
	}
}

func (r *reducer) nextItemID() string {
	r.itemSeq++
	return "item_" + r.cfg.SegmentID + "_" + strconv.Itoa(r.itemSeq)
}

func userMessageItemID(segmentID string) string { return "item_" + segmentID + "_u" }

func (r *reducer) open() ([]reduction, error) {
	createdAt := r.cfg.CreatedAt
	if createdAt.IsZero() {
		createdAt = r.now()
	}
	out := []RunEvent{SegmentStarted{Run: transcript.Run{
		ID: r.cfg.RunID, SessionID: r.cfg.SessionID,
		Provider: r.cfg.Provider, Model: r.cfg.Model,
		State: execution.Running, CreatedAt: createdAt, UpdatedAt: r.now(), MessageMark: -1,
	}}}
	out = append(out, r.openUserMessage()...)
	out = append(out, r.resumeQuestionCompletions()...)
	return r.project(out)
}

func (r *reducer) reduce(ev EngineEvent) ([]reduction, error) {
	var out []RunEvent
	switch e := ev.(type) {
	case TurnStart:
		return nil, nil
	case MessageDelta:
		out = r.closeReasoning()
		out = append(out, r.appendText(e.Text)...)
	case ReasoningDelta:
		out = r.closeText()
		out = append(out, r.appendReasoning(e.Text)...)
	case ToolCallStart:
		out = r.toolStart(e)
	case ToolCallEnd:
		out = r.toolEnd(e)
	case UsageReported:
		out = r.usageProgress(e)
	case SteerMessage:
		out = r.steerMessage(e)
	case TodosUpdated:
		out = r.todosSnapshot(e)
	case ErrorEvent:
		r.errMsg, r.errCode = e.Message, e.Code
		return nil, nil
	case CompactBoundary:
		out = r.compaction(e)
	case MemoryUpdated:
		return nil, nil
	case TurnInterrupted:
		var err error
		out, err = r.interrupt(e)
		if err != nil {
			return nil, fmt.Errorf("%w: interrupt: %w", errExecutorProtocol, err)
		}
	case TurnEnd:
		var err error
		out, err = r.turnEnd(e)
		if err != nil {
			return nil, fmt.Errorf("%w: turn end: %w", errExecutorProtocol, err)
		}
	default:
		return nil, fmt.Errorf("%w: unhandled event %T", errExecutorProtocol, ev)
	}
	return r.project(out)
}

func (r *reducer) synthesizeTerminal() ([]reduction, error) {
	out := r.closeStreaming()
	out = append(out, r.drainTools()...)
	result := &RunResult{}
	outcome := execution.OutcomeCanceled
	if r.errMsg != "" {
		outcome = execution.OutcomeError
		result.Error = r.classifyRunError(r.errMsg)
	}
	detail := ""
	if outcome == execution.OutcomeCanceled && r.cfg.CancelReason != nil {
		detail = r.cfg.CancelReason()
	}
	terminal, err := r.finishedRun(outcome, result, detail)
	if err != nil {
		return nil, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
	}
	out = append(out, terminal)
	return r.project(out)
}

func (r *reducer) abort(err error) {
	if err == nil {
		return
	}
	r.errMsg = err.Error()
	r.errCode = ""
}

func (r *reducer) project(events []RunEvent) ([]reduction, error) {
	out := make([]reduction, 0, len(events))
	for _, event := range events {
		reduced, err := r.projectOne(event)
		if err != nil {
			return nil, err
		}
		out = append(out, reduced)
	}

	// A park is one durable boundary: any drained/closed items, its running
	// approval/question items, open interrupt record, interrupted transcript run,
	// and admission transition must commit together before ANY event in this
	// reduction batch is published. Collapse every item projection into the park
	// write-set and mark the first reduction as the batch boundary; the pump then
	// commits and publishes the entire batch inside the cancel-linearization lock.
	interruptAt := -1
	for i := range out {
		if out[i].Interrupt {
			if interruptAt >= 0 {
				return nil, fmt.Errorf("%w: reduction batch has multiple interrupt boundaries", errReducerInvariant)
			}
			interruptAt = i
		}
		if itemStarted, ok := out[i].Event.(ItemStarted); ok {
			itemStarted.Item.SessionID = r.cfg.SessionID
			out[i].Event = itemStarted
		}
	}
	if interruptAt >= 0 {
		commit := out[interruptAt].Commit
		if commit == nil {
			return nil, fmt.Errorf("%w: interrupt boundary has no durable commit", errReducerInvariant)
		}
		items := make([]transcript.Item, 0, len(out))
		for i, reduced := range out {
			if i != interruptAt && reduced.Commit != nil {
				if reduced.Commit.Run != nil || reduced.Commit.Interrupt != nil || reduced.Commit.State != StateUnchanged {
					return nil, fmt.Errorf("%w: interrupt batch contains another lifecycle transition", errReducerInvariant)
				}
				items = append(items, reduced.Commit.Items...)
			}
			if itemStarted, ok := reduced.Event.(ItemStarted); ok {
				items = append(items, itemStarted.Item)
			}
			out[i].Commit = nil
			out[i].Interrupt = false
		}
		commit.Items = items
		out[0].Commit = commit
		out[0].Interrupt = true
	}
	if err := validateReductionBatch(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *reducer) projectOne(event RunEvent) (reduction, error) {
	commit := EventCommit{RunID: r.cfg.RunID, SessionID: r.cfg.SessionID}
	var nudge *Nudge
	switch e := event.(type) {
	case ItemCompleted:
		e.Item.SessionID = r.cfg.SessionID
		event = e
		commit.Items = []transcript.Item{e.Item}
		if e.Item.Status == ItemSucceeded && e.Item.Error == nil && len(e.mutatedPaths) > 0 {
			nudge = &Nudge{Cwd: r.cfg.Cwd, Paths: slices.Clone(e.mutatedPaths)}
		}
	case SegmentStarted:
		commit.Run = &e.Run
	case SegmentFinished:
		commit.Run = &e.Run
		if e.Run.State == execution.Interrupted {
			commit.Interrupt = &interrupts.Pending{
				RunID: r.cfg.RunID, SessionID: r.cfg.SessionID, TurnID: r.cfg.TurnID,
				Provider: r.cfg.Provider, Model: r.cfg.Model,
				Interrupts: e.Run.Interrupts, DrainedTools: r.drained,
				RunCreatedAt: r.cfg.CreatedAt, CreatedAt: r.now(),
			}
			commit.State = StateSuspend
			return reduction{Event: event, Commit: &commit, Interrupt: true}, nil
		}
		commit.State = StateTerminalize
		if e.Run.Outcome != nil {
			commit.Outcome = *e.Run.Outcome
		}
	case SegmentProgressed, ItemStarted, ItemChanged, StateSnapshot:
		// These events have no standalone EventCommit. Interrupt ItemStarted
		// projections are folded into the atomic park write-set by project.
	default:
		return reduction{}, fmt.Errorf("%w: unhandled run event %T", errReducerInvariant, event)
	}
	var durable *EventCommit
	if !commit.isEmpty() {
		durable = &commit
	}
	return reduction{Event: event, Commit: durable, Nudge: nudge}, nil
}

// validateReductionBatch checks the pump-facing shape before any commit or
// publication occurs. The reducer normally constructs this shape itself; the
// second check keeps future projection changes from creating partial durable
// boundaries.
func validateReductionBatch(reductions []reduction) error {
	if len(reductions) == 0 {
		return nil
	}

	interrupt := reductions[0].Interrupt
	terminalAt := -1
	for i, reduced := range reductions {
		if reduced.Event == nil {
			return fmt.Errorf("%w: reduction[%d] has no event", errReducerInvariant, i)
		}
		if reduced.Event.Terminal() {
			terminalAt = i
			if i != len(reductions)-1 {
				return fmt.Errorf("%w: terminal reduction[%d] is not last", errReducerInvariant, i)
			}
		}
		if reduced.Interrupt && i != 0 {
			return fmt.Errorf("%w: interrupt boundary at reduction[%d] must be first", errReducerInvariant, i)
		}
		if reduced.Commit != nil {
			switch reduced.Commit.State {
			case StateUnchanged:
			case StateSuspend:
				if !interrupt || i != 0 {
					return fmt.Errorf("%w: suspend commit at reduction[%d] has no interrupt boundary", errReducerInvariant, i)
				}
			case StateTerminalize:
				if interrupt || !reduced.Event.Terminal() {
					return fmt.Errorf("%w: terminal commit at reduction[%d] has no terminal event", errReducerInvariant, i)
				}
			default:
				return fmt.Errorf("%w: reduction[%d] has unknown state change %d", errReducerInvariant, i, reduced.Commit.State)
			}
		}
		if !interrupt {
			continue
		}
		if i > 0 && reduced.Commit != nil {
			return fmt.Errorf("%w: interrupt reduction[%d] repeats a durable commit", errReducerInvariant, i)
		}
	}

	if interrupt {
		commit := reductions[0].Commit
		switch {
		case commit == nil:
			return fmt.Errorf("%w: interrupt batch has no durable commit", errReducerInvariant)
		case commit.State != StateSuspend:
			return fmt.Errorf("%w: interrupt batch commit does not suspend the run", errReducerInvariant)
		case commit.Interrupt == nil:
			return fmt.Errorf("%w: interrupt batch commit has no pending interrupt", errReducerInvariant)
		case commit.Run == nil || commit.Run.State != execution.Interrupted:
			return fmt.Errorf("%w: interrupt batch commit has no interrupted run", errReducerInvariant)
		case terminalAt != len(reductions)-1:
			return fmt.Errorf("%w: interrupt batch has no terminal boundary event", errReducerInvariant)
		}
		return nil
	}

	if terminalAt < 0 {
		return nil
	}
	commit := reductions[terminalAt].Commit
	switch {
	case commit == nil:
		return fmt.Errorf("%w: terminal event has no durable commit", errReducerInvariant)
	case commit.State != StateTerminalize:
		return fmt.Errorf("%w: terminal event commit does not terminalize the run", errReducerInvariant)
	case commit.Run == nil || !commit.Run.State.IsTerminal():
		return fmt.Errorf("%w: terminal event commit has no terminal run", errReducerInvariant)
	case commit.Run.Outcome == nil || *commit.Run.Outcome != commit.Outcome:
		return fmt.Errorf("%w: terminal event commit has an inconsistent outcome", errReducerInvariant)
	}
	wantState, ok := execution.Running.Terminate(commit.Outcome)
	if !ok || commit.Run.State != wantState {
		return fmt.Errorf("%w: terminal event commit has an invalid lifecycle transition", errReducerInvariant)
	}
	return nil
}

func (r *reducer) now() time.Time { return r.cfg.Now().UTC() }
