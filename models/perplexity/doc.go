// Package perplexity wraps Perplexity's OpenAI-compatible Sonar API.
// Every Sonar model runs an online retrieval step before answering;
// the response includes citations + search results.
//
// Perplexity-specific knobs are exposed as [RequestOptions] through
// [RequestExtensionKey]:
//
//   - "search_mode" ("web" / "academic"), "search_domain_filter",
//     "search_recency_filter" steer the underlying web search.
//   - "return_images" / "return_related_questions" toggle extra
//     response fields.
//   - "web_search_options" controls per-call search behavior
//     (search context size, user location, etc.).
//
// Response extras (citations, search_results, images, related_questions,
// detailed usage, and costs) are preserved from the exact provider JSON in
// the namespaced Core response extension.
//
// See https://docs.perplexity.ai/ for the full API reference.
package perplexity
