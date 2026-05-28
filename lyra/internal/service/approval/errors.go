package approval

import "errors"

// ErrRequestNotFound is returned by [Service.Decide] when the
// supplied requestID does not match any pending request — either
// it never existed or it was already decided / canceled.
var ErrRequestNotFound = errors.New("approval: request not found")
