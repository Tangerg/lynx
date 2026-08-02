// Package options carries private helpers for decoding JSON-safe,
// extension-threaded OpenAI-wire params.
//
// A vendor's Extra metadata holds the JSON representation of its request
// extension under the package's modality-specific request extension key. GetParams decodes that
// value into T, returns a fresh zero value when absent, and surfaces malformed
// or type-incompatible JSON instead of silently discarding it.
//
// Internal: not part of the public API.
package options
