package safe

import (
	"fmt"
	"runtime/debug"
	"time"
)

// PanicError represents a recovered panic. It carries the panic value,
// the stack trace captured at recovery, and the timestamp.
type PanicError struct {
	time    time.Time
	info    any
	stack   []byte
	message string
}

// Error returns a multi-line message containing the timestamp, panic
// value, and stack trace.
func (e *PanicError) Error() string {
	return e.message
}

// NewPanicError builds a [*PanicError] from a recovered panic value and
// stack trace. The error message is formatted eagerly.
func NewPanicError(info any, stack []byte) error {
	now := time.Now()
	return &PanicError{
		time:  now,
		info:  info,
		stack: stack,
		message: fmt.Sprintf("panic recovered\ntimestamp: %s\nerror: %+v\nstack:\n%s",
			now.Format(time.RFC3339Nano), info, stack),
	}
}

// Go runs fn in a new goroutine. If fn panics, each handler is invoked
// with a [*PanicError] describing the panic. Handlers that themselves
// panic do not propagate.
//
// If fn is nil, Go does nothing.
//
// Example:
//
//	safe.Go(func() {
//	    process(req)
//	}, func(err error) {
//	    log.Printf("worker panic: %v", err)
//	})
func Go(fn func(), handlers ...func(error)) {
	wrapped := WithRecover(fn, handlers...)
	if wrapped == nil {
		return
	}
	go wrapped()
}

// WithRecover returns a function that runs fn with panic recovery.
// On panic, each handler is called with a [*PanicError]; if no handler
// is given, the panic is silently swallowed. Handler panics are also
// recovered and discarded.
//
// WithRecover returns nil if fn is nil. It is useful when you want
// recovery without spawning a goroutine.
func WithRecover(fn func(), handlers ...func(error)) func() {
	if fn == nil {
		return nil
	}
	return func() {
		defer func() {
			r := recover()
			if r == nil || len(handlers) == 0 {
				return
			}
			err := NewPanicError(r, debug.Stack())
			defer func() { _ = recover() }()
			for _, h := range handlers {
				h(err)
			}
		}()
		fn()
	}
}
