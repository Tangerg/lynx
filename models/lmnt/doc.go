// Package lmnt wraps LMNT's TTS API.
//
// [NewAudioTTSModel] targets LMNT's /v1/ai/speech endpoint. LMNT is
// optimized for ultra-low-latency synthesis (~300ms first-byte) on
// the Blizzard and Aurora voice families, with conversational
// pacing well suited to voice agents.
//
// Provider-specific knobs (speed, format, sample_rate, conversational
// mode, language, seed) ride through Extra-threaded SpeechRequest
// fields.
//
// See https://docs.lmnt.com/ for the full reference.
package lmnt
