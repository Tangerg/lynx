package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/event"
)

// publishEvent dispatches via the engine's multicast listener and
// the per-process multicast (populated from process-scope EventListener
// extensions). Either may be nil — the function tolerates that.
func (p *Process) publishEvent(ctx context.Context, e event.Event) {
	if p.engine != nil {
		p.engine.publishContext(ctx, e)
	}
	if p.processEvents != nil && e != nil {
		p.processEvents.OnEvent(ctx, e)
	}
}

// eventHeader stamps a fresh [event.Header] tagged with this process's
// id. Convenience used by every event the runtime emits — keeps the
// per-event struct literals one liner short.
func (p *Process) eventHeader() event.Header {
	return event.NewHeader(p.id)
}

// publishAny accepts the type-erased event used by ProcessContext.Emit.
func (p *Process) publishAny(ctx context.Context, e any) {
	ev, ok := e.(event.Event)
	if !ok {
		return
	}
	p.publishEvent(ctx, ev)
}
