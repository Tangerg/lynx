// Package elasticsearch wraps the official go-elasticsearch v8
// client as a [vectorstore.Store]. Documents are indexed JSON
// objects with a `dense_vector` field for the embedding and a
// nested `object` field for metadata.
//
// Requirements: Elasticsearch 8.0+ for dense_vector + `knn`
// top-level query. The store uses the `knn` query (not
// `script_score`) for retrieval — that's GA since 8.4.
//
// Similarity functions: [SimilarityCosine] / [SimilarityL2] /
// [SimilarityDotProduct]. The chosen value is recorded in the
// dense_vector mapping at index creation time and cannot be changed
// without rebuilding.
//
// Retrieval shape:
//
//	POST <index>/_search
//	{
//	  "size": K,
//	  "knn": {
//	    "field": "embedding",
//	    "query_vector": [...],
//	    "k": K,
//	    "num_candidates": ceil(K * NumCandidatesMultiplier),
//	    "filter": {"query_string": {"query": "<lucene>"}}
//	  }
//	}
//
// Filter visitor produces Lucene query-string syntax — metadata
// fields are addressed under `metadata.<key>` paths;
// LIKE wildcards (% / _) map to Lucene wildcards (* / ?).
//
// Delete uses _delete_by_query with the same Lucene filter.
//
// See https://www.elastic.co/docs/reference for the full API.
package elasticsearch
