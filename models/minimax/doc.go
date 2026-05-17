// Package minimax wraps MiniMax's chat APIs.
//
// MiniMax operates two billing zones (international USD / domestic RMB)
// and exposes the chat surface in two compatibility flavors:
//
//   - OpenAI-compatible at /v1 — use [NewOpenAIChatModel];
//   - Anthropic-compatible at /anthropic — use [NewAnthropicChatModel],
//     which routes through the [anthropic] provider so the Anthropic
//     SDK's tool-calling, extended thinking, and reasoning-signature
//     handling all work as-is.
//
// MiniMax-specific surfaces (Text-to-Speech, Voice Clone, Image
// generation, Video generation) are separate endpoints with custom
// wire formats not exposed by this package.
//
// See https://platform.minimaxi.com/document for the full API.
package minimax
