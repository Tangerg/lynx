// Package fireworks wraps Fireworks AI's OpenAI-compatible API.
// Fireworks hosts open-weight models on its FireAttention serving
// stack and ships latency-optimized custom variants of popular
// models.
//
// Fireworks-specific knobs reachable via Extra-threaded openai params:
//
//   - "context_length_exceeded_behavior" controls truncation policy.
//   - "prompt_cache_max_len" enables Fireworks' prompt-cache layer.
//   - The /chat/completions endpoint accepts "response_format" with
//     "type":"grammar" to constrain output via GBNF (alongside the
//     standard "json_schema").
//
// See https://docs.fireworks.ai/ for the full API reference.
package fireworks
