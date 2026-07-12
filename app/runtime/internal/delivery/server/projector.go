package server

import (
	"context"
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

func (p *projector) Open() []runs.ProjectedEvent {
	// The opening run.started carries no terminal, so classification is moot.
	return p.project(p.tr.open(), false, 0)
}

func (p *projector) Translate(ev runs.EngineEvent) []runs.ProjectedEvent {
	// The run-lifecycle classification (does this event end/park the run, with
	// what outcome) comes from the engine-neutral event's domain contract
	// ([execution.Event]) — NOT re-derived from the wire projection below. The
	// concrete turn event is asserted back only to shape the wire timeline; a
	// non-turn event can't reach here (the facade is the sole executor), so an
	// unexpected type is a wiring bug — drop it rather than fabricate a projection.
	outcome, _ := ev.Terminal()
	interrupt := ev.Interrupt()
	te, ok := ev.(turn.Event)
	if !ok {
		return nil
	}
	return p.project(p.tr.translate(te), interrupt, outcome)
}

// SynthesizeTerminal builds the terminal for a stream that ended without one:
// errored when the translator recorded a failure (an ErrorEvent or an Abort from
// a failed commit), else canceled. Never a park — a stream that drained left no
// live turn to resume. The protocol outcome shapes the wire payload; the domain
// [execution.Outcome] the commit's Terminalize.
func (p *projector) SynthesizeTerminal() []runs.ProjectedEvent {
	if p.tr.errMsg != "" {
		return p.project(p.tr.finish(protocol.OutcomeError), false, execution.OutcomeError)
	}
	return p.project(p.tr.finish(protocol.OutcomeCanceled), false, execution.OutcomeCanceled)
}

// Abort records a commit-failure message; the next SynthesizeTerminal reports
// the run as errored.
func (p *projector) Abort(msg string) { p.tr.errMsg = msg }

// project turns the translator's wire events into projected events, stamping the
// caller's run-lifecycle classification (parks + terminal outcome) onto the
// run.finished frame among them.
func (p *projector) project(events []protocol.StreamEvent, parks bool, outcome execution.Outcome) []runs.ProjectedEvent {
	out := make([]runs.ProjectedEvent, 0, len(events))
	for _, se := range events {
		pe := p.projected(se, parks, outcome)
		out = append(out, pe)
		if pe.Abort {
			break
		}
	}
	return out
}

// projected maps one wire event to a projected event. Durability/terminal-frame
// are read off the wire type (delivery's own structured output); the lifecycle
// COMMIT the frame carries is supplied by the caller from the engine event's
// domain contract, not re-derived from the wire outcome: a park grows its
// resumable record (marshal failure aborts the run) + a Suspend transition; a
// plain terminal grows a Terminalize transition with the domain outcome; a
// canceled terminal picks up the late-bound cancel reason from the segment view.
// The run-state transition rides the SAME EventCommit as the record it must agree
// with, so the commit is atomic (§8.3).
func (p *projector) projected(se protocol.StreamEvent, parks bool, outcome execution.Outcome) runs.ProjectedEvent {
	terminal := se.Type == protocol.StreamRunFinished
	interrupt := terminal && parks
	if terminal && !interrupt && outcome == execution.OutcomeCanceled && se.Outcome != nil && se.Outcome.Detail == "" {
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
		commit.Outcome = outcome
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
// or subscription drop) OR the request ctx is canceled (client disconnect),
// closing the wire channel in turn. The ctx-aware send is load-bearing: without
// it a disconnected client whose downstream stops reading would block the mapper
// forever on an in-flight send — the source closing can't unblock a stuck send —
// leaking one goroutine per disconnect.
func mapRunEvents(ctx context.Context, in <-chan runs.Event) <-chan protocol.RunEvent {
	out := make(chan protocol.RunEvent)
	go func() {
		defer close(out)
		for e := range in {
			se, _ := e.Payload.(protocol.StreamEvent)
			ev := protocol.RunEvent{
				RunID: e.RunID,
				// The application cursor is opaque + prefix-free; delivery owns the
				// evt_ wire framing (§11.2). Fixed-width so lexical == numeric order.
				EventID:   protocol.IDPrefixEvent + e.Seq,
				Timestamp: e.Timestamp,
				Event:     se,
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
