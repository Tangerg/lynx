// Package event defines the framework's typed lifecycle events and concurrent
// multicast listener. Runtime publishers and subscribers exchange Event values
// directly; listeners may use a type switch when they need an event-specific
// payload.
//
// Every event type implements [encoding/json.Marshaler] and produces a
// self-describing JSON object — useful for audit logs, federation, and
// observability sinks that want raw payloads. Marshaling is one-way:
// interface-typed fields ([core.Action], [core.WorldState],
// [planning.Plan], [error]) collapse to lossy summary forms (a name
// string, a state map, …). Round-trip deserialization is intentionally
// not provided — listeners that need it should consume events in their
// in-memory form.
package event
