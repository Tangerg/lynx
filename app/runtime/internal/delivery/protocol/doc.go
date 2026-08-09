// Package protocol is the single source of truth for the Lyra Runtime Protocol.
// Its typed interfaces and values define the behavior and wire shapes shared
// by transports and protocol implementations.
//
// doc/API.md describes the wire semantics; contract/API_REFERENCE.md is the
// generated method index for [Runtime]. The model is Session → Run → Item
// (doc/API.md §0): Item is the single history+streaming primitive, Runs finish
// with a discriminated RunOutcome, and human-in-the-loop finishes one Segment
// with an interrupt outcome before resuming the same Run in a new Segment.
//
// Discriminated unions (StreamEvent, Item, RunOutcome, ItemDelta, and Interrupt)
// are flat tag-discriminated structs. One `type` field selects the optional
// fields its variant permits; `kind` never appears on the wire.
package protocol
