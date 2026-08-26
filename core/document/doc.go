// Package document defines the canonical serializable [Document] content
// value shared by extraction, retrieval, and model-facing components.
//
// NewDocument requires text, media, or both. Metadata is JSON-safe and belongs
// to the document itself; query-specific relevance belongs to the vector-store
// match value. Extraction, formatting, splitting, identifier assignment,
// batching, and loading policy live in the separate etl module.
package document
