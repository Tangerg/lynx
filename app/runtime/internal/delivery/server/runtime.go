package server

// RuntimePort is the inbound adapter's consumer-side port into the runtime
// application boundary. It is composed from the server's four bounded use-case
// contexts rather than from a list of individual methods. The protocol layer
// therefore stays free of an internal/runtime import and a future remote
// runtime (or a test fake) can satisfy the same surface.
//
// *internal/runtime.Runtime satisfies this implicitly; the composition
// root (cmd/lyra) passes the concrete value where a RuntimePort is
// expected.
type RuntimePort interface {
	sessionUseCases
	turnUseCases
	settingsUseCases
	workspaceUseCases
}
