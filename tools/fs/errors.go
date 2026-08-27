package fs

import "errors"

// Sentinel errors surfaced by executors and tool plumbing.
var (
	// ErrEmptyPath is returned when an input Path is empty.
	ErrEmptyPath = errors.New("fs: path must not be empty")

	// ErrEmptyPattern is returned when Glob/Grep input has no pattern.
	ErrEmptyPattern = errors.New("fs: pattern must not be empty")

	// ErrRipgrepUnavailable is returned when local content search cannot find
	// the required ripgrep executable.
	ErrRipgrepUnavailable = errors.New("fs: ripgrep is unavailable")

	// ErrBinaryFile is returned by Read/Edit when the target file
	// looks binary. Only text files are supported.
	ErrBinaryFile = errors.New("fs: file appears to be binary; only text files are supported")
)
