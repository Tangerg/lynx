// Package protocol defines the binding-neutral public values of the ScopeApp
// Runtime Protocol. HTTP and the embedded Go binding use these same requests,
// responses, events, errors, version values, and strict validators.
//
// This package deliberately contains no server method interface, context key,
// transport envelope, numeric JSON-RPC code, Host, Store, or execution handle.
// Consumers define their own narrow interfaces; the concrete embedded Runtime
// is published by package embedded.
//
// doc/API.md describes the semantics and contract/API_REFERENCE.md is the
// generated operation index. The model is Session → Run → Item: Item is the
// history and streaming primitive, and human-in-the-loop ends one Segment with
// an interrupt before the same Run resumes in another Segment.
package protocol
