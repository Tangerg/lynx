// Package vectorstore is the unified abstraction layer over vector
// databases (Pinecone, Weaviate, Milvus, Qdrant, Chroma, ...). Three
// composable interfaces split the surface by responsibility:
//
//   - [Creator] writes documents.
//   - [Retriever] finds similar documents by query + metadata filter.
//   - [Deleter] removes documents matching a metadata filter.
//
// The full [Store] interface composes all three. Provider
// packages live under github.com/Tangerg/lynx/vectorstores/<name>.
//
// Request shapes:
//
//   - [CreateRequest], [RetrievalRequest], [DeleteRequest] — each has
//     a Validate() method enforcing required fields.
//   - Sentinel errors [ErrNilRequest], [ErrEmptyDocuments], and
//     [ErrMissingFilter] let callers errors.Is the validation
//     failures.
//
// Metadata filtering uses the filter mini-language: build expressions
// programmatically with filter.NewExprBuilder or parse from text with
// filter.Parse. See [github.com/Tangerg/lynx/core/vectorstore/filter]
// for grammar and examples.
//
// Quick start:
//
//	expr, _ := filter.ParseAndAnalyze(`category == "tech" AND year >= 2020`)
//	req, _ := vectorstore.NewRetrievalRequest("attention", 5, 0.7, expr)
//	docs, err := store.Retrieve(ctx, req)
//
// To bridge a vector store into a document writer for ingest
// pipelines, see [NewDocumentWriter].
package vectorstore
