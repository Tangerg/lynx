// Package docio holds the document-side glue every vector store
// re-implements: UUID fallback for the document id, JSON metadata
// (un)marshaling, and the textual `[v1,v2,...]` vector literal a
// handful of SQL backends accept in lieu of a typed binding.
package docio
