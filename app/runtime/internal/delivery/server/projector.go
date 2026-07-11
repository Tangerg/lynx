package server

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
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
	// sideEffect derives the durable record for a wire event; it closes over the
	// run's ids + cwd + createdAt so this type stays free of the Server.
	sideEffect func(protocol.StreamEvent) runsegment.Event
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
// record (marshal failure aborts the run); a canceled terminal picks up the
// late-bound cancel reason from the segment view.
func (p *projector) projected(se protocol.StreamEvent) runs.ProjectedEvent {
	terminal := se.Type == protocol.StreamRunFinished
	interrupt := terminal && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeInterrupt
	if terminal && !interrupt && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeCanceled && se.Outcome.Detail == "" {
		if reason := p.view.CancelReason(); reason != "" {
			se.Outcome.Detail = reason
		}
	}
	effect := p.sideEffect(se)
	if interrupt {
		raw, err := json.Marshal(se.Outcome.Interrupts)
		if err != nil {
			return runs.ProjectedEvent{Abort: true}
		}
		effect.Interrupt = &runsegment.Interrupt{
			RunID:        p.runID,
			Handle:       p.handle,
			Provider:     p.provider,
			Model:        p.model,
			Payload:      raw,
			DrainedTools: p.tr.parkDrained,
		}
	}
	return runs.ProjectedEvent{
		Durable:   se.IsDurable(),
		Terminal:  terminal,
		Interrupt: interrupt,
		Payload:   se,
		Effect:    effect,
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
			sideEffect: func(se protocol.StreamEvent) runsegment.Event {
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
				RunID:     e.RunID,
				EventID:   e.Seq,
				Timestamp: e.Timestamp,
				Event:     se,
			}
		}
	}()
	return out
}

// cursorMinter adapts the Server's wire event-id source to the Coordinator's
// CursorMinter port (evt_ minting stays a delivery concern).
type cursorMinter struct{ next func() string }

func (c cursorMinter) Mint() string { return c.next() }
