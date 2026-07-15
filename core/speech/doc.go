// Package speech defines the stable text-to-speech protocol and independent
// synchronous [Model] and optional [Streamer] provider capabilities.
//
// NewRequest captures text and Options carries explicit voice, format, speed,
// and provider-specific overrides. Streamer is separate from Model so consumers
// only require streaming from implementations that actually support it.
package speech
