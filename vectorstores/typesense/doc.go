// Package typesense wraps Typesense's vector search as a
// [vectorstore.Store]. Documents are regular Typesense documents in
// a collection with id / content / metadata (nested object) / embedding
// (float[]) fields, reached through the official typesense-go v3
// client.
//
// Requirements: Typesense 0.25+ (vector search GA) — the store uses
// nested-object metadata which needs `enable_nested_fields=true` on
// the collection.
//
// Distance metric: cosine only. Typesense's vector search always uses
// cosine distance — the result `vector_distance` is in [0, 2] and the
// store maps it onto a higher-is-better score in [0, 1].
//
// Schema bootstrap. When [StoreConfig.InitializeSchema] is true the
// store probes for the collection and creates it with the right
// fields + dimensionality if missing. Existing collections are
// trusted as-is.
//
// Filter visitor produces Typesense `filter_by` syntax — `metadata.k:=
// v`, `metadata.year:>= 2020`, `metadata.tag:= [a,b]` (IN form). The
// metadata field is a nested object so keys are addressed under the
// configured prefix.
//
// NOT caveat. Typesense `filter_by` has no top-level NOT operator —
// the visitor rewrites `NOT (x op y)` into the operator's inverse
// (e.g. `NOT (year >= 2020)` → `metadata.year:< 2020`). NOT wrapping
// anything other than a single binary comparison is rejected.
//
// See https://typesense.org/docs/latest/api/vector-search.html.
package typesense
