package fs

import "errors"

// Sentinel errors surfaced by executors and tool plumbing.
var (
	// ErrEmptyPath is returned when an input Path is empty.
	ErrEmptyPath = errors.New("fs: path must not be empty")

	// ErrEmptyPattern is returned when Glob/Grep input has no pattern.
	ErrEmptyPattern = errors.New("fs: pattern must not be empty")

	// ErrBinaryFile is returned by Read/Edit when the target file
	// looks binary. Only text files are supported.
	ErrBinaryFile = errors.New("fs: file appears to be binary; only text files are supported")

	// ErrNotFound is returned when a file or directory does not exist.
	// Implementations should also wrap os.ErrNotExist for callers using
	// errors.Is.
	ErrNotFound = errors.New("fs: not found")
)
