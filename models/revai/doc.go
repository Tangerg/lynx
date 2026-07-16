// Package revai wraps Rev AI's speech-to-text API.
//
// [NewAudioTranscriptionModel] orchestrates Rev AI's async /v1/jobs
// flow (submit → poll → fetch). Rev AI's strength is enterprise-
// grade accuracy on English / multilingual content plus rich
// metadata (speaker channels, custom vocabularies, profanity
// filter, language ID).
//
// Provider extras (speaker_channels_count, custom_vocabulary_id,
// language, remove_disfluencies, transcriber selection) ride through
// extension-threaded JobOptions fields.
//
// See https://docs.rev.ai/ for the full reference.
package revai
