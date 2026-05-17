// Package redis wraps Redis Stack's RediSearch module as a
// [vectorstore.Store]. Documents are stored as Redis HASHes keyed at
// `<KeyPrefix><id>`; an FT.CREATE-defined index registers the
// vector field plus any pre-declared metadata fields.
//
// Requirements: Redis Stack (or Redis OSS 8.0+ with the search
// module) — RediSearch is mandatory. RedisJSON is NOT required;
// the store deliberately uses HASH storage to keep the dependency
// surface minimal.
//
// Distance metrics: [DistanceCosine] / [DistanceL2] / [DistanceIP].
// Vector index algorithm: [AlgorithmHNSW] (default) / [AlgorithmFlat].
//
// Metadata model. Every filterable metadata key MUST be declared in
// [StoreConfig.MetadataFields] up-front with its RediSearch type —
// [FieldTag] (exact match), [FieldNumeric] (range queries), or
// [FieldText] (full-text). Filters against undeclared fields fail
// fast via [ErrUnknownMetadataField] (rather than reaching Redis
// and silently producing zero hits).
//
// Query path. The filter visitor emits RediSearch syntax — TAG
// `@f:{v}`, NUMERIC `@f:[low high]`, TEXT `@f:(v)`. Vector retrieval
// runs FT.SEARCH with the hybrid syntax
// `(<filter>)=>[KNN K @embedding $vec AS distance]`, passing the
// binary FLOAT32 little-endian vector through PARAMS.
//
// See https://redis.io/docs/latest/develop/interact/search-and-query/
// for the RediSearch reference.
package redis
