// Package openrouter wraps OpenRouter — a unified gateway that routes
// to 100+ LLMs across 50+ providers through a single OpenAI-compatible
// API.
//
// Model ids use a "provider/model-name" format (e.g.
// "anthropic/claude-3.5-sonnet", "openai/gpt-4o",
// "google/gemini-2.5-pro", "deepseek/deepseek-chat:free"). Suffixes
// like ":free" / ":nitro" / ":floor" select cost/latency variants.
//
// OpenRouter-specific features the openai facade plumbs through
// transparently:
//
//   - models array for automatic fallback across alternatives
//     (set chat.Options.Extra["openai_params"]
//     ChatCompletionNewParams.WithModels(...));
//   - provider preference routing (provider field in extra body);
//   - transforms (middle-out compression).
//
// This facade adds typed knobs for the two app-attribution headers
// OpenRouter asks integrations to set (HTTP-Referer and X-Title) so
// the calling app shows up on the OpenRouter leaderboard.
//
// See https://openrouter.ai/docs for the full docs.
package openrouter
