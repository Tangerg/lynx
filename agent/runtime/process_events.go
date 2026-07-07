package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/event"
)

// publishEvent dispatches via the platform's multicast listener and
// the per-process multicast (populated from process-scope EventListener
// extensions). Either may be nil — the function tolerates that.
func (p *AgentProcess) publishEvent(ctx context.Context, e event.Event) {
	if p.platform != nil {
		p.platform.publishContext(ctx, e)
	}
	if p.processEvents != nil && e != nil {
		p.processEvents.OnEventContext(ctx, e)
	}
}

// baseEvent stamps a fresh [event.BaseEvent] tagged with this process's
// id. Convenience used by every event the runtime emits — keeps the
// per-event struct literals one liner short.
func (p *AgentProcess) baseEvent() event.BaseEvent {
	return event.NewBaseEvent(p.id)
}

// publishAny accepts the type-erased event used by ProcessContext.Publish.
func (p *AgentProcess) publishAny(e any) {
	p.publishAnyContext(context.Background(), e)
}

func (p *AgentProcess) publishAnyContext(ctx context.Context, e any) {
	ev, ok := e.(event.Event)
	if !ok {
		return
	}
	p.publishEvent(ctx, ev)
}
