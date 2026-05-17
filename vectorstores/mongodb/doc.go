// Package mongodb wraps MongoDB Atlas Vector Search as a
// [vectorstore.Store]. Documents are stored as ordinary BSON
// documents (`{_id, content, metadata, embedding}`); retrieval runs
// the `$vectorSearch` aggregation stage.
//
// Requirements: MongoDB Atlas (vector search isn't available on
// self-hosted Community / Enterprise — it's an Atlas-only feature).
// The store uses the v2 official driver
// (go.mongodb.org/mongo-driver/v2).
//
// Vector similarity functions: [SimilarityCosine] /
// [SimilarityEuclidean] / [SimilarityDotProduct]. The chosen value
// is recorded in the Atlas Vector Search index definition.
//
// Indexes. Atlas Vector Search indexes are NOT regular MongoDB
// indexes; they're managed via the Search Indexes API and live on
// dedicated Atlas search nodes. The store creates one automatically
// under [StoreConfig.InitializeSchema] = true, including any
// metadata fields enumerated in
// [StoreConfig.MetadataFieldsToFilter] as typed `filter` paths.
//
// Filter visitor produces MongoDB query-document syntax —
// `{"metadata.author": {"$eq": "Alice"}}`, `{"$and": [...]}`,
// `{"$nor": [...]}` for NOT, `{"$regex": ..., "$options": "i"}` for
// LIKE. The result feeds the `filter` field of `$vectorSearch`.
//
// Retrieval pipeline:
//
//	{$vectorSearch: {...}}, {$addFields: {score: {$meta: "vectorSearchScore"}}},
//	{$match: {score: {$gte: minScore}}}
//
// See https://www.mongodb.com/docs/atlas/atlas-vector-search/.
package mongodb
