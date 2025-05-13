package sync

import "github.com/Tangerg/lynx/pkg/safe"

// Go same to safe.GO.
func Go(fn func(), errfns ...func(error)) {
	safe.Go(fn, errfns...)
}
