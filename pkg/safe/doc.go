// Package safe provides panic-recovery helpers for goroutines.
//
// [Go] launches a goroutine that recovers from panics and forwards them
// to user-supplied handlers. [WithRecover] wraps a function with the
// same recovery logic without spawning a goroutine. Recovered panics
// are reported as a [*PanicError] containing the panic value, stack
// trace, and timestamp.
package safe
