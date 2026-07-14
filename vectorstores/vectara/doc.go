// Package vectara wraps Vectara's managed RAG service as a
// the vectorstore capability interfaces. Vectara handles embedding, chunking, and
// retrieval internally — the store sends raw text to the v2 API and
// does NOT need an [embedding.Model]. This is unlike every other
// lynx vector store.
//
// Requirements: a Vectara account, an API key with corpus-level
// write + query scope, and a corpus provisioned via the Vectara
// console or control-plane API. The embedder, retrieval model, and
// chunking strategy are configured on the corpus itself.
//
// Authentication. API key via the `x-api-key` header.
//
// Retrieval shape. The store hits Vectara's v2 query endpoint —
// `POST /v2/corpora/<corpus_key>/query` — with the user's raw query
// and a `metadata_filter` string derived from the filter visitor.
// Scores come from Vectara directly (higher = more similar).
//
// Filter visitor produces Vectara's metadata-filter SQL-like syntax
// — `doc.author = 'Alice'`, `doc.year >= 2020`, `doc.tag IN ('a',
// 'b')`, `NOT (...)`, ` AND ` / ` OR `. Metadata keys are addressed
// under the `doc.` prefix by default; pass [StoreConfig.MetadataPrefix]
// = `"part"` to filter part-level metadata instead.
//
// Documents are uploaded as `type: "core"` with a single
// `document_parts` entry holding the raw text — Vectara does its own
// chunking on the server side.
//
// Delete. Vectara has no bulk filter-delete; the store enumerates
// matching ids via the list endpoint (paged via `page_key`) and
// issues per-id DELETEs against `/v2/corpora/<corpus_key>/documents/
// <doc_id>`.
//
// See https://docs.vectara.com/docs/rest-api/.
package vectara
