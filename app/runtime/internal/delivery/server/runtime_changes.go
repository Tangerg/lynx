package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// observeChanges installs the delivery side of the composition-root change bridge:
// each committed application mutation becomes the one invalidation signal that says
// its resource moved (§7.3). The application has already committed — Delivery only
// chooses the wire vocabulary, which is the whole reason the producers name a
// resource instead of a topic.
func (s *Server) observeChanges(src Source[change.Notice]) {
	src.Observe(func(notice change.Notice) {
		if ev, ok := runtimeEventFor(notice); ok {
			s.PublishRuntimeEvent(ev)
		}
	})
}

// runtimeEventFor projects a committed change onto its runtime event. The ID sets a
// signal may carry are per-topic (§7.3): goals.changed is session-addressed, and a
// session-scope state key must not carry run ids — a client that narrowed a refetch
// by a run id there would be narrowing by something the key is not keyed on.
//
// An unmapped resource is reported rather than guessed at; TestEveryChangeResourceIsPublishable
// keeps the closed set honest, so ok=false is a build error caught in tests, not a
// silent hole in production.
func runtimeEventFor(notice change.Notice) (protocol.RuntimeEvent, bool) {
	switch notice.Resource {
	case change.Sessions:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeSessionsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case change.Runs:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeRunsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case change.Interrupts:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeInterruptsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case change.Goals:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeGoalsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case change.PlanState:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeStateChanged, Key: protocol.StatePlan, SessionIDs: notice.SessionIDs,
		}, true
	default:
		return protocol.RuntimeEvent{}, false
	}
}
