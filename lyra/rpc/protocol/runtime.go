// Package protocol is the single source of truth for the Lyra Runtime
// Protocol — the typed Go interface every transport and every
// implementation agrees on. The wire formats (JSON-RPC over HTTP /
// Wails IPC / InProcess) are derived from this surface; the runtime
// implementation in rpc/server realises it on top of Lyra's
// internal engine + service layer.
//
// The docs at docs/{API,TRANSPORT}.md describe the wire protocol;
// every method on [Runtime] corresponds to one row in API.md §5.2.
//
// Design notes:
//
//   - The interface is composed from per-domain sub-interfaces
//     (Lifecycle, Sessions, Messages, Runs, ...). Mocks in tests can
//     stub one slice without re-implementing the universe.
//   - Methods that have no backing yet return [ErrNotImplemented].
//     The dispatch translates that to JSON-RPC -32601 method_not_found
//     so the wire stays honest.
//   - Streaming methods (runs.start, workspace.terminal.subscribe,
//     background.subscribe) return a Go channel of events together with
//     a sync result. Transports translate the channel into transport-
//     specific notification streams (SSE / Wails EventsEmit / Go chan).
package protocol

// Runtime is the runtime's public surface — the union of every
// method group exposed over the wire. See sub-interfaces for the
// per-domain method signatures.
//
// Construct one via pkg/server.New(...) and pass to any transport
// adapter (rpc/transport/inprocess, rpc/transport/http).
type Runtime interface {
	Lifecycle
	Sessions
	Messages
	Runs
	Workspace
	Providers
	Models
	Tools
	Attachments
	Background
	Feedback
}
