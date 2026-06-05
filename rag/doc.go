// Package rag implements a five-stage Retrieval-Augmented Generation
// pipeline: query transform → expand → retrieve → refine → augment.
// Each stage is pluggable; concrete implementations live alongside the
// interfaces ([QueryTransformer], [QueryExpander], [DocumentRetriever],
// [DocumentRefiner], [QueryAugmenter]). Wire them together with
// [PipelineConfig] and run via [Pipeline.Execute].
//
// Quick start:
//
//	pipe, err := rag.NewPipeline(rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{retriever},
//	    QueryAugmenter:     contextual,
//	})
//	q, _ := rag.NewQuery("what is GOAP?")
//	augmented, docs, err := pipe.Execute(ctx, q)
//
// Stage 3 (retrieval) runs every retriever in parallel and unions the
// results; partial failures keep the docs already collected. The other
// stages are sequential. See [Pipeline.Execute] for the per-stage
// error-wrapping shape.
//
// Drop-in [Nop] satisfies every stage interface — useful as a default
// when a stage is optional or as a test double.
//
// To wire RAG into a chat client as middleware (so retrieved context
// is folded into every chat call), see [NewPipelineMiddleware].
//
// # Multi-retriever fan-out
//
// Lynx deliberately does NOT ship a separate "DocumentJoiner"
// abstraction. When multiple retrievers run in parallel, the pipeline
// unions their result lists into a flat slice, and the refine stage is
// where you re-organize that slice. The canonical "join overlapping
// retriever results" pattern is the refiner pair below — equivalent to
// spring-ai's ConcatenationDocumentJoiner:
//
//	pipe, _ := rag.NewPipeline(rag.PipelineConfig{
//	    DocumentRetrievers: []rag.DocumentRetriever{vectorR1, vectorR2},
//	    DocumentRefiners: []rag.DocumentRefiner{
//	        rag.NewDeduplicationRefiner(), // drop duplicate IDs
//	        rag.NewRankRefiner(topK),      // sort by score desc + cap
//	    },
//	})
//
// This works for any number of retrievers whose scores live on the
// same scale (e.g. all are vector-similarity stores). Reciprocal Rank
// Fusion (RRF) — the rank-based fusion algorithm — only adds value
// when retriever scores are NOT comparable (BM25 mixed with dense
// vectors, sparse mixed with hybrid, etc.). Until lynx ships a non-
// vector retriever, RRF doesn't solve a real problem; a dedicated
// DocumentJoiner abstraction will land at that point so it can be
// designed against real constraints rather than guessed.
//
// # Per-query retriever routing
//
// Likewise, there is no "QueryRouter" stage. To route a query to a
// subset of retrievers (e.g. by topic, language, or metadata), wrap
// your retrievers in a custom [DocumentRetriever] that switches on
// the query internally:
//
//	type routingRetriever struct {
//	    docsR, logsR rag.DocumentRetriever
//	}
//	func (r *routingRetriever) Retrieve(ctx context.Context, q *rag.Query) ([]*document.Document, error) {
//	    if route, _ := q.Get("route"); route == "logs" {
//	        return r.logsR.Retrieve(ctx, q)
//	    }
//	    return r.docsR.Retrieve(ctx, q)
//	}
//
// Pipeline stays oblivious; routing logic lives where it belongs (the
// retriever boundary).
package rag
