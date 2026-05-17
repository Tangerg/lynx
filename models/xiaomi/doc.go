// Package xiaomi wraps Xiaomi's MiMo API open platform.
//
// MiMo serves the MiMo model family (V2-flash / V2-pro / V2-omni /
// V2.5 / V2.5-pro) at two compatibility flavors on the same host:
//
//   - OpenAI-compatible at /v1 — use [NewOpenAIChatModel];
//   - Anthropic-compatible at /anthropic — use [NewAnthropicChatModel],
//     which routes through the [anthropic] provider so the Anthropic
//     SDK's tool-calling, extended thinking, and reasoning-signature
//     handling all work as-is.
//
// Provider-specific features the openai facade plumbs through
// transparently:
//
//   - thinking mode on reasoning-capable models (mimo-v2.5-pro,
//     mimo-v2-pro). Enable by setting
//     [chat.Options].Extra["openai_params"] to a
//     ChatCompletionNewParams whose Body carries
//     {"thinking": {"type": "enabled"}}. The reasoning_content field
//     in the response is auto-surfaced as
//     chat.AssistantMessage.Reasoning;
//   - 1M-token context window on V2.5-series models.
//
// MiMo-specific surfaces not exposed here (TTS / image / omni I/O)
// require provider-specific request shapes that don't map onto the
// OpenAI chat-completions wire. Use the platform's dedicated
// endpoints directly for those.
//
// See https://platform.xiaomimimo.com/docs for the full API
// reference.
package xiaomi
