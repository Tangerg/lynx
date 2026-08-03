// Package vectorstore defines provider-neutral semantic indexing and search.
// Four independent interfaces split the surface by capability:
//
//   - [Indexer] adds documents.
//   - [Searcher] finds similar documents by query + metadata filter.
//   - [IDDeleter] removes documents by identifier.
//   - [FilterDeleter] removes documents matching a metadata filter.
//   - [Batcher] supplies an order-preserving ingestion partition policy.
//
// There is deliberately no aggregate Store interface: consumers depend only
// on the capabilities they call, and providers implement only what they can
// support. Batching remains an injected capability rather than a framework
// dependency. [ValidateDocuments] and [BatchDocuments] enforce the shared
// ingestion boundary before provider I/O. [SearchRequest] validates both input
// and successful Match output; Add and delete methods accept their single
// logical input directly.
//
// Indexed documents have a caller-assigned, non-empty ID and non-empty text.
// Providers preserve both values so every successful Match is immediately
// usable by retrieval pipelines. Providers never generate IDs, and a
// vector-index-plus-external-document-store architecture must hydrate results
// explicitly outside these capabilities rather than return partial Documents.
// Provider distances and similarities are converted to the common [0, 1]
// contract with the Normalize functions in this package.
//
// Metadata filtering uses the filter mini-language: build predicates with
// typed constructors or parse them from text with filter.Parse. See
// [github.com/Tangerg/lynx/core/vectorstore/filter].
//
// Quick start:
//
//	expr, _ := filter.Parse(`category == 'tech' AND year >= 2020`)
//	req := vectorstore.SearchRequest{
//		Query: "attention", TopK: 5, MinScore: 0.7, Filter: expr,
//	}
//	matches, err := searcher.Search(ctx, req)
package vectorstore
