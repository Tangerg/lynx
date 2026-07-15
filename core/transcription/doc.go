// Package transcription defines the serializable audio-to-text protocol and
// its single-method [Model] capability.
//
// NewRequest accepts validated media audio. Options carries explicit language,
// prompt, temperature, response format, timestamp granularity, and provider
// extras written through Options.Set so they are JSON-safe. Request has no
// arbitrary parameter bag. Implementations and defaults live outside Core.
package transcription
