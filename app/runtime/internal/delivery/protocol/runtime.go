// Package protocol is the single source of truth for the Lyra Runtime
// Protocol v2 — the typed Go interface every transport and every
// implementation agrees on. Wire formats (JSON-RPC over HTTP / IPC /
// InProcess) are derived from this surface; delivery/server realizes it on
// top of Lyra's internal kernel + domain layer.
//
// ../desktop/docs/protocol/API.md describes the wire contract; every method on [Runtime]
// maps to one row in API.md §7. The model is Session → Run → Item
// (API.md §0): Item is the single history+streaming primitive, runs
// finish with a discriminated RunOutcome, and human-in-the-loop uses
// the R model (finish with interrupt outcome, resume via a
// continuation run).
//
// Discriminated unions (StreamEvent / Item / RunOutcome / ItemDelta /
// Interrupt) are modeled as flat tag-discriminated
// structs: a single `type` discriminator field plus the optional
// fields that tag declares (API.md §2.1: one discriminator `type`,
// `kind` never appears on the wire). The wire JSON is exactly
// {type, ...}, matching API.md.
package protocol

import "time"

// Runtime is the runtime's public surface — the union of every method
// group exposed over the wire. Construct via delivery/server.New(...) and
// pass to any transport adapter.
type Runtime interface {
	Lifecycle
	Sessions
	Runs
	Items
	Workspace
	Skills
	Recipes
	AgentDocs
	MCP
	Hooks
	Approval
	Schedules
	Goals
	Codebase
	Providers
	Models
	Tools
	Memory
	AgentMemory
	Feedback
	UsageReports
}

// ProtocolVersion is the wire version this build implements (API.md
// §12: date string).
const (
	ProtocolVersion    = "2026-07-19"
	MinProtocolVersion = "2026-07-19"
)

type ProtocolRange struct {
	Current      string `json:"current"`
	MinSupported string `json:"minSupported"`
}

func SupportedProtocolRange() ProtocolRange {
	return ProtocolRange{Current: ProtocolVersion, MinSupported: MinProtocolVersion}
}

func SupportsProtocolVersion(version string) bool {
	if _, err := time.Parse(time.DateOnly, version); err != nil {
		return false
	}
	return version >= MinProtocolVersion && version <= ProtocolVersion
}

// Resource id prefixes (API.md §2.2). Server-generated, type-tagged.
const (
	IDPrefixSession = "ses_"
	IDPrefixRun     = "run_"
	IDPrefixSegment = "seg_"
	IDPrefixItem    = "item_"
	IDPrefixEvent   = "evt_"
)
