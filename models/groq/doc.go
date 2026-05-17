// Package groq wraps Groq's OpenAI-compatible API. Groq runs open-
// weight models (Llama, Gemma, DeepSeek, Kimi) on its in-house LPUs
// at extremely high throughput.
//
// Groq-specific knobs reachable via Extra-threaded openai params:
//
//   - service_tier ("on_demand" / "flex" / "auto") trades cost for
//     latency. See https://console.groq.com/docs/flex-processing.
//   - reasoning_format ("parsed" / "raw" / "hidden") controls how
//     reasoning-model output is surfaced.
//
// See https://console.groq.com/docs/ for the full API reference.
package groq
