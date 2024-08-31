package sync

import (
	"fmt"
	"runtime/debug"
	"time"
)

func Go(fn func(), errfn ...func(err error)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				timestamp := time.Now().Format(time.RFC3339Nano)
				stackTrace := string(debug.Stack())

				err := fmt.Errorf("panic recovered: %v\nTimestamp: %s\nStack Trace:\n%s", r, timestamp, stackTrace)
				for _, f := range errfn {
					f(err)
				}
			}
		}()
		fn()
	}()
}
