// Package perplexity wraps Perplexity's OpenAI-compatible Sonar API.
// Every Sonar model runs an online retrieval step before answering;
// the response includes citations + search results.
//
// Perplexity-specific knobs reachable through the namespaced OpenAI request
// extension:
//
//   - "search_mode" ("web" / "academic"), "search_domain_filter",
//     "search_recency_filter" steer the underlying web search.
//   - "return_images" / "return_related_questions" toggle extra
//     response fields.
//   - "web_search_options" controls per-call search behavior
//     (search context size, user location, etc.).
//
// Response extras (citations, search_results, related_questions) come
// back as provider response fields and are preserved in namespaced
// core/chat response extensions when mapped by the OpenAI adapter.
//
// See https://docs.perplexity.ai/ for the full API reference.
package perplexity
