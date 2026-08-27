package shell

import "errors"

var ErrEmptyCommand = errors.New("shell: command must not be empty")
