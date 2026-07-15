// Package vectorstore defines provider-neutral semantic indexing and search.
// Four independent interfaces split the surface by capability:
//
//   - [Indexer] adds documents.
//   - [Searcher] finds similar documents by query + metadata filter.
//   - [IDDeleter] removes documents by identifier.
//   - [FilterDeleter] removes documents matching a metadata filter.
//
// There is deliberately no aggregate Store interface: consumers depend only
// on the capabilities they call, and providers implement only what they can
// support. [SearchRequest] validates both input and successful Match output;
// Add and delete methods accept their single logical input directly.
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
