// Package anthropic wraps Anthropic's Messages API and OpenAI-
// compatible bridge.
//
// Constructors:
//
//   - [NewChatModel] — native /v1/messages. Full Claude surface:
//     extended thinking blocks, tool_use with signature continuity,
//     citations, fine-grained tool-result content blocks,
//     cache_control;
//   - [NewOpenAIChatModel] — Anthropic's first-party OpenAI-compat
//     bridge ([BaseURLOpenAI]). Wire-format-only conversion for
//     callers wedded to the OpenAI SDK; Claude-specific extras
//     don't round-trip.
//
// Token estimation: [NewTextEstimator] wraps /v1/messages/count_tokens
// for accurate Claude-tokenizer-based counts.
//
// Anthropic's Message Batches API (~50% pricing, up to 24h
// asynchronous) doesn't fit core/model's request/response shape and
// is not exposed.
//
// Model id constants aren't exported — anthropic-sdk-go owns them
// ([anthropicsdk.ModelClaudeOpus4_5], [anthropicsdk.ModelClaudeSonnet4_5],
// [anthropicsdk.ModelClaudeHaiku4_5], etc.). Import the SDK directly
// when you need them.
//
// See https://docs.claude.com/en/api for the full API reference.
package anthropic
