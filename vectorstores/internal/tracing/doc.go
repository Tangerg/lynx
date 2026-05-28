// Package tracing centralizes the OTel span emission shared by every
// vector-store provider in this module. Each provider's Create /
// Retrieve / Delete entry point wraps its inner SDK work with the
// helpers here so the GenAI / DB semconv attribute set stays
// consistent across the 27-provider matrix.
//
// Per doc/OBSERVABILITY.md §3.2 the attributes follow the OTel DB
// semconv plus the lynx vector-specific extensions:
//
//	db.system                            — provider id ("qdrant", "pgvector", ...)
//	db.operation.name                    — "create" / "retrieve" / "delete"
//	db.vector.query.top_k                — RetrievalRequest.TopK
//	db.vector.query.similarity_threshold — RetrievalRequest.MinScore
//	lynx.rag.doc_count                   — result size (retrieve) or input size (create)
package tracing
