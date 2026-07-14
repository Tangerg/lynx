// Package milvus wraps Milvus / Zilliz Cloud as a
// the vectorstore capability interfaces. Documents are stored as rows in a Milvus
// collection (`{id, content, embedding, <metadata columns>}`);
// retrieval runs Milvus's ANN search.
//
// Requirements: a reachable Milvus 2.x server (self-hosted, Docker,
// or Zilliz Cloud managed service). The store uses the official
// milvus-sdk-go/v2 gRPC client.
//
// Vector similarity functions: cosine / L2 / IP. The chosen value
// is bound to the collection's index at creation time; switching
// requires rebuilding the index.
//
// Schema. Milvus is strongly typed — every metadata field that
// participates in filters must be declared as a typed column at
// schema-creation time. [StoreConfig.MetadataFields] enumerates the
// columns; anything outside that set goes into a flexible JSON
// field that can still be filtered but at a higher cost.
//
// Filter visitor produces Milvus's expression language —
// `author == "Alice" and (year > 2020 or tag in ["a","b"])`. The
// result feeds the `expr` parameter of the search call.
//
// See https://milvus.io/docs for the full API surface.
package milvus
