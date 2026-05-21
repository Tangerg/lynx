// Package event defines the framework's lifecycle event types and the
// multicast Listener that ferries them to subscribers. Events are
// type-erased to "any" by the runtime when published so core can stay
// independent of this package; type-asserting listeners switch on the
// concrete struct.
//
// Every event type implements [encoding/json.Marshaler] and produces a
// self-describing JSON object — useful for audit logs, federation, and
// observability sinks that want raw payloads. Marshaling is one-way:
// interface-typed fields ([core.Action], [core.WorldState],
// [core.Awaitable], [error]) collapse to lossy summary forms (a name
// string, a state map, …). Round-trip deserialization is intentionally
// not provided — listeners that need it should consume events in their
// in-memory form.
//
// File organisation (post-split):
//
//   - event.go       — Event interface + BaseEvent + envelope/emit helpers
//   - multicast.go   — Listener + ListenerFunc + Multicast
//   - platform.go    — Agent (un)deployed events
//   - process.go     — Process lifecycle events
//   - planning.go    — Planner-related events
//   - execution.go   — Action execution + goal achievement events
//   - summaries.go   — Internal wire-shape structs for Goal/Plan/WorldState/Awaitable
package event
