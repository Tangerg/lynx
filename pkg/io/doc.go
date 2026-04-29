// Package io extends the standard library's io package with helpers
// that allow callers to control the read-buffer size.
//
// [ReadAll] reads everything into one buffer; [Read] yields chunks via
// a Go 1.23 iterator and is suitable for streaming.
package io
