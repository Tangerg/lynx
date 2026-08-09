// Package protocol is the single source of truth for the Lyra Runtime Protocol.
// Its typed interfaces and values define the behavior and wire shapes shared
// by transports and protocol implementations.
//
// doc/API.md describes the wire semantics; contract/API_REFERENCE.md is the
// generated method index for [Runtime]. The model is Session → Run → Item
// (doc/API.md §0): Item is the single history+streaming primitive, runs
// finish with a discriminated RunOutcome, and human-in-the-loop uses
// the R model (finish one segment with an interrupt outcome, then resume the
// same run in a new segment).
//
// Discriminated unions (StreamEvent / Item / RunOutcome / ItemDelta /
// Interrupt) are modeled as flat tag-discriminated
// structs: a single `type` discriminator field plus the optional
// fields that tag declares (doc/API.md §2.1: one discriminator `type`,
// `kind` never appears on the wire). The wire JSON is exactly
// {type, ...}, matching the generated contract schema.
package protocol

import "time"

// Runtime is the runtime's public surface: the union of every method group
// exposed over the wire.
type Runtime interface {
	Lifecycle
	Sessions
	Runs
	Items
	Plan
	RuntimeSubscription
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
	Knowledge
	AgentMemory
	Feedback
	UsageReports
}

// ProtocolVersion is the wire version this build implements (doc/API.md
// §12: date string).
//
// Current and minimum supported are deliberately the same date: this build
// serves exactly [ProtocolVersion]. A range wider than one version would
// advertise a negotiation the code does not perform.
const (
	ProtocolVersion    = "2026-08-09"
	MinProtocolVersion = "2026-08-09"
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

// Resource id prefixes (doc/API.md §2.2). Server-generated, type-tagged.
const (
	IDPrefixSession = "ses_"
	IDPrefixRun     = "run_"
	IDPrefixSegment = "seg_"
	IDPrefixItem    = "item_"
	IDPrefixEvent   = "evt_"
)
