// Package gladia wraps Gladia's speech-to-text API.
//
// [NewAudioTranscriptionModel] orchestrates Gladia's async pipeline
// (upload → /v2/transcription submit → poll → fetch). Gladia's
// strengths are multilingual transcription (100+ languages with
// automatic code-switching), speaker diarization, and add-ons like
// summarization, translation, named-entity recognition, and audio
// intelligence — all reachable via Extra-threaded
// TranscriptionRequest fields.
//
// See https://docs.gladia.io/ for the full reference.
package gladia
