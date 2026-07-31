// Package xiaomi wraps Xiaomi's MiMo API open platform.
//
// MiMo serves the current V2.5 and V2.5 Pro models at two compatibility
// flavors on the same host:
//
//   - OpenAI-compatible at /v1 — use [NewOpenAIChat];
//   - Anthropic-compatible at /anthropic — use [NewAnthropicChat],
//     which routes through the [anthropic] provider so the Anthropic
//     SDK's tool-calling, extended thinking, and reasoning-signature
//     handling all work as-is.
//
// Provider-specific thinking is configured with [ChatRequestOptions] under
// [RequestExtensionKey]. reasoning_content is surfaced as Core reasoning and
// replayed on tool-call turns as required by MiMo's protocol.
//
// MiMo-specific surfaces not exposed here (TTS / image / omni I/O)
// require provider-specific request shapes that don't map onto the
// OpenAI chat-completions wire. Use the platform's dedicated
// endpoints directly for those.
//
// See https://mimo.mi.com/docs for the full API
// reference.
package xiaomi
