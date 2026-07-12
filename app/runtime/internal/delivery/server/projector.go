package server

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// projector is the delivery-side implementation of the run Coordinator's
// Projector port: it wraps the stateful wire translator + the durable
// side-effect derivation so the application pump stays wire-agnostic. One per
// run segment (the translator carries in-flight item state).
type projector struct {
	tr       *translator
	view     runs.SegmentView
	runID    string
	provider string
	model    string
	handle   turn.TurnHandle
	// sideEffect derives a wire event's durable transcript commit + non-durable
	// nudge; it closes over the run's ids + cwd + createdAt so this type stays
	// free of the Server.
	sideEffect func(protocol.StreamEvent) (execution.EventCommit, *runs.Nudge)
}

var _ runs.Projector = (*projector)(nil)

func (p *projector) Open() []runs.ProjectedEvent { return p.project(p.tr.open()) }

func (p *projector) Translate(ev runs.EngineEvent) []runs.ProjectedEvent {
	// The executor yields turn events; the port is engine-neutral so the pump
	// stays wire-agnostic. A non-turn event can't reach here (the facade is the
	// sole executor), so an unexpected type is a wiring bug — drop it rather than
	// fabricate a projection.
	te, ok := ev.(turn.Event)
	if !ok {
		return nil
	}
	return p.project(p.tr.translate(te))
}

// SynthesizeTerminal builds the terminal for a stream that ended without one.
// The outcome is error when the translator recorded a failure (an ErrorEvent or
// an Abort from a failed commit), else canceled.
func (p *projector) SynthesizeTerminal() []runs.ProjectedEvent {
	outcome := protocol.OutcomeCanceled
	if p.tr.errMsg != "" {
		outcome = protocol.OutcomeError
	}
	return p.project(p.tr.finish(outcome))
}

// Abort records a commit-failure message; the next SynthesizeTerminal reports
// the run as errored.
func (p *projector) Abort(msg string) { p.tr.errMsg = msg }

func (p *projector) project(events []protocol.StreamEvent) []runs.ProjectedEvent {
	out := make([]runs.ProjectedEvent, 0, len(events))
	for _, se := range events {
		pe := p.projected(se)
		out = append(out, pe)
		if pe.Abort {
			break
		}
	}
	return out
}

// projected maps one wire event to a projected event: durability/terminal are
// pure functions of the wire type; the interrupt terminal grows its resumable
// record (marshal failure aborts the run) + a Suspend transition; a plain
// terminal grows a Terminalize transition carrying the mapped outcome; a canceled
// terminal picks up the late-bound cancel reason from the segment view. The
// run-state transition rides the SAME EventCommit as the record it must agree
// with, so the commit is atomic (§8.3).
func (p *projector) projected(se protocol.StreamEvent) runs.ProjectedEvent {
	terminal := se.Type == protocol.StreamRunFinished
	interrupt := terminal && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeInterrupt
	if terminal && !interrupt && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeCanceled && se.Outcome.Detail == "" {
		if reason := p.view.CancelReason(); reason != "" {
			se.Outcome.Detail = reason
		}
	}
	commit, nudge := p.sideEffect(se)
	switch {
	case interrupt:
		pending, err := p.interruptPending(se)
		if err != nil {
			return runs.ProjectedEvent{Abort: true}
		}
		commit.Interrupt = pending
		commit.State = execution.StateSuspend
	case terminal:
		commit.State = execution.StateTerminalize
		commit.Outcome = outcomeFromWire(se.Outcome)
	}
	return runs.ProjectedEvent{
		Durable:   se.IsDurable(),
		Terminal:  terminal,
		Interrupt: interrupt,
		Payload:   se,
		Commit:    commitOrNil(commit),
		Nudge:     nudge,
	}
}

// interruptPending builds the resumable record for a parked terminal. ProcessID
// is left empty — the runsegment adapter resolves it from the live turn inside
// the commit — so this stays a pure wire→domain projection. A marshal failure
// returns an error, aborting the run.
func (p *projector) interruptPending(se protocol.StreamEvent) (*interrupts.Pending, error) {
	raw, err := json.Marshal(se.Outcome.Interrupts)
	if err != nil {
		return nil, err
	}
	return &interrupts.Pending{
		ParentRunID:  p.runID,
		SessionID:    p.handle.SessionID,
		TurnID:       p.handle.TurnID,
		Provider:     p.provider,
		Model:        p.model,
		Interrupts:   raw,
		DrainedTools: p.tr.parkDrained,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// commitOrNil drops an empty commit (a Live delta persists nothing) to nil so the
// pump skips the durable write.
func commitOrNil(c execution.EventCommit) *execution.EventCommit {
	if c.Item == nil && c.Run == nil && c.Interrupt == nil && c.State == execution.StateUnchanged {
		return nil
	}
	return &c
}

// outcomeFromWire maps the wire terminal outcome onto the durable
// [execution.Outcome] recorded on the run's admission row. An interrupt is not an
// outcome (it is the Interrupted state, handled via Suspend); a missing outcome
// defaults to completed.
func outcomeFromWire(o *protocol.RunOutcome) execution.Outcome {
	if o == nil {
		return execution.OutcomeCompleted
	}
	switch o.Type {
	case protocol.OutcomeCanceled:
		return execution.OutcomeCanceled
	case protocol.OutcomeError:
		return execution.OutcomeError
	case protocol.OutcomeMaxSteps:
		return execution.OutcomeMaxSteps
	case protocol.OutcomeMaxBudget:
		return execution.OutcomeMaxBudget
	default:
		return execution.OutcomeCompleted
	}
}

// segmentProjector builds the per-segment projector factory the Coordinator
// calls once it has a segment view. It captures everything the wire translation
// + durable derivation need, so the application pump only sees the port.
func (s *Server) segmentProjector(runID, parentRunID, sessionID, cwd string, handle turn.TurnHandle, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string, createdAt time.Time) func(runs.SegmentView) runs.Projector {
	return func(view runs.SegmentView) runs.Projector {
		return &projector{
			tr:       newTranslator(sessionID, runID, parentRunID, userInput, resume, provider, model),
			view:     view,
			runID:    runID,
			provider: provider,
			model:    model,
			handle:   handle,
			sideEffect: func(se protocol.StreamEvent) (execution.EventCommit, *runs.Nudge) {
				return sideEffectEvent(runID, sessionID, parentRunID, cwd, se, provider, model, createdAt)
			},
		}
	}
}

// mapRunEvents adapts the Coordinator's transport-neutral event stream to the
// wire RunEvent channel the protocol returns, attaching the run/cursor/timestamp
// envelope. The mapping goroutine ends when the source closes (terminal teardown
// or subscription drop), closing the wire channel in turn.
func mapRunEvents(in <-chan runs.Event) <-chan protocol.RunEvent {
	out := make(chan protocol.RunEvent)
	go func() {
		defer close(out)
		for e := range in {
			se, _ := e.Payload.(protocol.StreamEvent)
			out <- protocol.RunEvent{
				RunID: e.RunID,
				// The application cursor is opaque + prefix-free; delivery owns the
				// evt_ wire framing (§11.2). Fixed-width so lexical == numeric order.
				EventID:   protocol.IDPrefixEvent + e.Seq,
				Timestamp: e.Timestamp,
				Event:     se,
			}
		}
	}()
	return out
}
