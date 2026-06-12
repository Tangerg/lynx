// Package moonshot wraps Moonshot AI's Kimi chat APIs.
//
// Moonshot serves the same models at two compatibility flavors and
// two billing regions:
//
//   - OpenAI-compatible at /v1 — use [NewOpenAIChatModel];
//   - Anthropic-compatible at /anthropic — use [NewAnthropicChatModel],
//     supported on Kimi-K2 and newer reasoning models. Allows
//     Anthropic-SDK callers to swap base URL.
//
// Use [BaseURL] / [BaseURLAnthropic] for the domestic Chinese region
// and [BaseURLIntl] / [BaseURLIntlAnthropic] for the international
// (api.moonshot.ai) host.
//
// See https://platform.moonshot.cn/docs for the docs.
package moonshot
