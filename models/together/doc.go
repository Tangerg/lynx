// Package together wraps Together AI's OpenAI-compatible API.
// Together hosts hundreds of open-weight models (Llama, DeepSeek,
// Qwen, Mistral, etc.) with serverless and dedicated endpoints.
//
// Together-specific knobs reachable via Extra-threaded openai params:
//
//   - "echo" / "n" / "min_p" / "repetition_penalty" / "top_k" are
//     accepted on top of the standard openai surface.
//   - The "safety_model" field enables Llama Guard prefilter / postfilter
//     by naming a guard model.
//
// See https://docs.together.ai/ for the full reference.
package together
