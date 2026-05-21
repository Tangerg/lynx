// Package model defines the request/response handler primitives that every AI
// modality (chat, embedding, image, audio, moderation) builds on top of.
// The two core abstractions are [CallHandler] for synchronous request-response
// and [StreamHandler] for incremental output, both parameterized over arbitrary
// Request and Response types.
package model
