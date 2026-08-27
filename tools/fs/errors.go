package fs

import "errors"

var (
	ErrEmptyPath = errors.New("fs: path must not be empty")

	ErrEmptyPattern = errors.New("fs: pattern must not be empty")

	ErrRipgrepUnavailable = errors.New("fs: ripgrep is unavailable")

	ErrBinaryFile = errors.New("fs: file appears to be binary; only text files are supported")
)
