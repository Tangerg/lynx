// Package image defines the serializable image-generation protocol and its
// single-method [Model] capability.
//
// NewRequest captures the prompt; Options carries explicit per-call overrides
// such as dimensions, quality, output MIME type, and response representation.
// Provider-only options use Options.Set so Extra remains JSON-safe; Request has
// no arbitrary parameter bag. Implementations and defaults live outside Core.
package image
