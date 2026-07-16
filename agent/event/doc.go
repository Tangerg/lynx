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
// [planning.Plan], [error]) collapse to lossy summary forms (a name
// string, a state map, …). Round-trip deserialization is intentionally
// not provided — listeners that need it should consume events in their
// in-memory form.
//
// File organization (post-split):
//
//   - event.go        — Event interface and Header
//   - multicast.go    — Listener, ListenerFunc, and Multicast
//   - deployment.go   — agent deployment events
//   - process.go      — process lifecycle events
//   - planning.go     — planning events
//   - action.go       — action and goal events
//   - interaction.go  — managed-interaction boundaries
//   - usage.go        — model and embedding usage events
//   - json*.go        — stable JSON envelopes and summary shapes
package event
