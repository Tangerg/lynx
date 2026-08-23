// Package settings contains the persistence-neutral vocabulary shared by
// approval and schedule use cases.
package settings

import "errors"

var ErrNotFound = errors.New("settings: resource not found")
