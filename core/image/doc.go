// Package image defines the serializable image-generation protocol and its
// single-method [Model] capability.
//
// NewRequest captures the prompt; Options carries explicit per-call overrides
// such as dimensions, quality, output MIME type, and response representation.
// Provider implementations and defaults live outside Core in provider modules.
package image
