// Package protocol is the single source of truth for the Lyra Runtime
// Protocol v2 — the typed Go interface every transport and every
// implementation agrees on. Wire formats (JSON-RPC over HTTP / IPC /
// InProcess) are derived from this surface; rpc/server realizes it on
// top of Lyra's internal engine + service layer.
//
// docs/API.md describes the wire contract; every method on [Runtime]
// maps to one row in API.md §7. The model is Session → Run → Item
// (API.md §0): Item is the single history+streaming primitive, runs
// finish with a discriminated RunOutcome, and human-in-the-loop uses
// the R model (finish with interrupt outcome, resume via a
// continuation run).
//
// Discriminated unions (StreamEvent / Item / ToolInvocation /
// RunOutcome / ItemDelta / ContextItem) are modeled as flat
// tag-discriminated structs: a Type/Kind field plus the optional
// fields that tag declares. The wire JSON is exactly {type, ...},
// matching API.md.
package protocol

// Runtime is the runtime's public surface — the union of every method
// group exposed over the wire. Construct via rpc/server.New(...) and
// pass to any transport adapter.
type Runtime interface {
	Lifecycle
	Sessions
	Runs
	Items
	Workspace
	Providers
	Models
	Tools
	Memory
	Attachments
	Background
	Feedback
}

// ProtocolVersion is the wire version this build implements (API.md
// §11: date string).
const ProtocolVersion = "2026-06-03"

// Resource id prefixes (API.md §2.2). Server-generated, type-tagged.
const (
	IDPrefixSession    = "ses_"
	IDPrefixRun        = "run_"
	IDPrefixItem       = "item_"
	IDPrefixAttachment = "att_"
	IDPrefixTask       = "tsk_"
	IDPrefixEvent      = "evt_"
)
