package workspace

import "errors"

var (
	ErrFileListTooLarge = errors.New("workspace: file listing too large")
	ErrPageLimit        = errors.New("workspace: page limit invalid")
	ErrPageCursor       = errors.New("workspace: page cursor invalid")
	ErrVCSUnavailable   = errors.New("workspace: VCS unavailable")
	ErrVCSBaseUnknown   = errors.New("workspace: VCS base unknown")
)
