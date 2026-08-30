// Package rerank defines the provider-neutral protocol for ranking documents
// against one query. Results address the immutable input batch by index so the
// protocol does not duplicate document ownership or depend on a retrieval type.
package rerank
