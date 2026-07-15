// Package document defines the serializable [Document] content value and the
// minimal [Reader] and [Writer] I/O vocabulary.
//
// NewDocument requires text, media, or both. Metadata is JSON-safe and belongs
// to the document itself; query-specific relevance belongs to the vector-store
// match value. Loading, splitting, enrichment, batching, retries, and pipeline
// policy live in the separate documentpipeline module.
package document
