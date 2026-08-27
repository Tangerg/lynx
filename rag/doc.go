// Package rag provides small interfaces and combinators for
// Retrieval-Augmented Generation.
//
// Quick start:
//
//	q, _ := rag.NewQuery("what is GOAP?")
//	docs, err := rag.Retrieve(ctx, retriever, q)
//
// The package owns the stable contracts ([Transformer], [Expander],
// [Retriever], [Refiner], and [Augmenter]) as well as the small concrete
// adapters that make those contracts useful: vector-store retrieval,
// model-backed query transforms and reranking, citation-aware contextual
// augmentation, and chat middleware. Keeping them together follows the Go
// standard-library style:
// one discoverable package, small interfaces, explicit composition.
//
// Composition is explicit. Wrap a retriever with the stages you need:
//
//	r, err := rag.WithTransformers(base, rewrite, translate)
//	r, err = rag.WithExpander(r, multiQuery)
//	top, err := rag.TopK(8)
//	r, err = rag.WithRefiners(r, top)
//	docs, err := r.Retrieve(ctx, q)
//
// Optional stages use identity implementations: [IdentityTransformer],
// [IdentityExpander], [NopRetriever], [IdentityRefiner], and
// [IdentityAugmenter].
//
// # Parallel retriever fan-out
//
// Lynx deliberately does not ship a separate "DocumentJoiner"
// abstraction. Use [Parallel] to run retrievers concurrently and union their
// result lists into a flat slice; use refiners to re-organize that slice.
// A typical "join overlapping retriever results" pattern is:
//
//	top, err := rag.TopK(topK)
//	combined, err := rag.Parallel(vectorR1, vectorR2)
//	r, err := rag.WithRefiners(combined, top)
//
// TopK keeps the highest-scoring candidate for each non-empty document ID
// before ranking and capping, so duplicate hits cannot consume result slots.
// Use [Dedup] separately only when unique documents are needed without score
// ordering or a result cap. Score-based refiners assume all retrievers use a
// comparable score scale. Use [ReciprocalRankFusion] before TopK when combining
// unlike ranking systems so fusion depends on result order instead of raw
// scores.
//
// # Per-query retriever routing
//
// Likewise, there is no "QueryRouter" stage. To route a query to a
// subset of retrievers (e.g. by topic, language, or metadata), wrap
// your retrievers in a custom [Retriever] that switches on the query
// internally:
//
//	var routeKey = rag.MustValueKey[string]("route")
//
//	type routingRetriever struct {
//	    docsR, logsR rag.Retriever
//	}
//	func (r *routingRetriever) Retrieve(ctx context.Context, q *rag.Query) ([]rag.Candidate, error) {
//	    route, _, err := q.Value(routeKey)
//	    if err != nil {
//	        return nil, err
//	    }
//	    if route == "logs" {
//	        return r.logsR.Retrieve(ctx, q)
//	    }
//	    return r.docsR.Retrieve(ctx, q)
//	}
//
// Callers stay oblivious; routing logic lives where it belongs (the
// retriever boundary).
package rag
