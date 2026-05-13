package bash

import "errors"

// ErrEmptyCommand is returned when [RunInput.Cmd] is empty.
var ErrEmptyCommand = errors.New("bash: command must not be empty")
