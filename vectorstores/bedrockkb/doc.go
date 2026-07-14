// Package bedrockkb wraps AWS Bedrock Knowledge Bases as a semantic searcher.
// Bedrock Knowledge Base is a managed RAG
// service — embedding, chunking, and persistence are all handled
// behind the API; lynx only consumes the runtime Retrieve surface.
//
// Requirements: an AWS account with Bedrock Knowledge Bases enabled,
// a provisioned knowledge base wired to a data source (S3, Confluence,
// SharePoint, Salesforce, etc.), and an aws-sdk-go-v2
// bedrockagentruntime client.
//
// Document lifecycle. Bedrock ingests via the configured data source
// + StartIngestionJob — there's no runtime upsert / delete. The store exposes
// no fake mutation methods. Manage documents via the data source instead
// (StartIngestionJob through the bedrockagent control plane).
//
// Retrieve uses Bedrock's runtime Retrieve API with the configured
// [types.KnowledgeBaseVectorSearchConfiguration] — NumberOfResults
// is populated from [vectorstore.SearchRequest.TopK]. Callers can
// override search type / reranking / implicit filter via
// [StoreConfig.VectorSearchOverrides].
//
// Filter visitor produces [types.RetrievalFilter] — Bedrock's typed
// filter shape (Equals / NotEquals / GreaterThan / LessThan /
// GreaterThanOrEquals / LessThanOrEquals / StringContains / In /
// NotIn / AndAll / OrAll / etc.).
//
// Identifiers. Bedrock retrieval results don't expose stable per-row
// ids; the store mints `Document.ID` from the result's `Location`
// (e.g. the S3 URI of the source object) and falls back to the content
// text if Location is empty.
//
// See https://docs.aws.amazon.com/bedrock/latest/userguide/knowledge-base.html.
package bedrockkb
