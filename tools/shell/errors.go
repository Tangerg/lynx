package shell

import "errors"

// ErrEmptyCommand is returned when [Input.Cmd] is empty.
var ErrEmptyCommand = errors.New("shell: command must not be empty")
