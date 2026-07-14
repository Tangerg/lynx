// Package pinecone wraps Pinecone as a the vectorstore capability interfaces.
// Documents are stored as vectors in a Pinecone index
// (`{id, values, metadata}`); retrieval runs the index's similarity
// query.
//
// Requirements: a Pinecone account and an existing index (created
// via the Pinecone console or control-plane API — Pinecone does not
// allow lazy index creation from the data plane). The store uses
// the official pinecone-io/go-pinecone v4 client.
//
// Vector similarity. Pinecone configures cosine / dotproduct /
// euclidean at index-creation time; the store reads but does not
// override.
//
// Filter visitor produces Pinecone's metadata-filter syntax —
// `{"author": {"$eq": "Alice"}}`, `{"$and": [...]}`,
// `{"$in": [...]}`. The result feeds the `Filter` field of the
// query request. Pinecone has no native LIKE / regex; the visitor
// rejects [filter.OpLike] expressions explicitly.
//
// Document text. Pinecone itself stores only id + vector + flat
// metadata — there is no first-class text body. The store stashes
// the original document text under a reserved metadata key when
// [StoreConfig.StoreDocumentContent] is true; retrieval reverses
// the mapping back into [document.Document.Text].
//
// See https://docs.pinecone.io/ for the full API surface.
package pinecone
