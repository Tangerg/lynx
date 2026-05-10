// Package rag implements a five-stage Retrieval-Augmented Generation
// pipeline: query transform → expand → retrieve → refine → augment.
// Each stage is pluggable; concrete implementations live alongside the
// interfaces ([QueryTransformer], [QueryExpander], [DocumentRetriever],
// [DocumentRefiner], [QueryAugmenter]). Wire them together with
// [PipelineConfig] and run via [Pipeline.Execute].
//
// Quick start:
//
//	pipe, err := rag.NewPipeline(&rag.PipelineConfig{
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
package rag
