// Package xml is a streaming XML element scanner geared at LLM
// output processing — extracting structured `<think>` / `<tool_use>`
// / custom tags from a token stream as they arrive, not after the
// stream closes.
//
// [StreamScanner] consumes bytes incrementally and fires
// [ElementListener] callbacks as soon as each registered element
// closes. Listeners can cap per-element buffer size so a runaway
// tag in the model output can't blow up memory.
//
// The package complements (does not replace) [encoding/xml]: stdlib
// is best when the document is well-formed and you have it in
// memory; this package is best for partial / streaming / LLM-tainted
// input where you want known tags pulled out and the rest passed
// through verbatim.
package xml
