// Package deepgram wraps Deepgram's speech APIs.
//
// Two modalities are exposed:
//
//   - /v1/listen via [NewAudioTranscriptionModel] — high-throughput
//     real-time / batch transcription on the Nova family. Provider-
//     specific knobs (smart_format, diarize, utterances, paragraphs,
//     redact, keywords) ride through Extra-threaded params;
//   - /v1/speak via [NewAudioTTSModel] — synthesis on the Aura voice
//     family.
//
// Deepgram's live streaming (WebSocket) and analyze surfaces aren't
// modeled here — the WebSocket flow doesn't fit core/model's request/
// response shape.
//
// See https://developers.deepgram.com/docs for the full reference.
package deepgram
