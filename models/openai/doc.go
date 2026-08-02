// Package openai exposes OpenAI's native chat, Responses, embedding, image,
// moderation, speech, transcription, and translation adapters.
//
// Every constructor returns an OpenAI-owned model type. Reusable wire behavior
// remains private to the models module, so compatible providers do not leak
// OpenAI concrete types through their APIs.
package openai
