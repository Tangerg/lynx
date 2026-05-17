// Package azurecosmos wraps Azure Cosmos DB for NoSQL's
// VectorDistance() function as a [vectorstore.Store]. Documents are
// regular Cosmos items (`{id, content, metadata, embedding}`) and
// retrieval runs a parameterised SQL query that orders rows by
// VectorDistance.
//
// Requirements: a Cosmos DB account with vector search enabled
// (currently a feature flag on the NoSQL API; opt in from the
// portal). The container needs a vector embedding policy + indexing
// policy that match the configured [DistanceFunction] and the
// embedding model's dimensionality — the store assumes this is
// provisioned out of band (ARM / Terraform / Portal).
//
// Distance functions: [DistanceCosine] / [DistanceDotProduct] /
// [DistanceEuclidean]. The value passed at query time MUST match
// what the container's vector policy declares.
//
// Filter visitor produces Cosmos SQL — `c.metadata.key = @p1`,
// `c.metadata.year >= @p1`, `c.metadata.tag IN (@p1, @p2)`. Named
// parameters (`@pN`) are used to match Cosmos SDK's QueryParameter
// shape. LIKE maps to `CONTAINS(c.metadata.key, @p)` — the leading
// / trailing `%` markers are stripped.
//
// Cross-partition queries are enabled by passing the canonical empty
// partition key to NewQueryItemsPager. Single-doc writes use the
// document's id as partition key (matches the default `/id` config).
//
// See https://learn.microsoft.com/azure/cosmos-db/nosql/vector-search.
package azurecosmos
