// Package runtime is Lyra's core-runtime facade; one struct that bundles the
// kernel and every domain service a transport adapter might need. The
// architecture goal documented in GREENFIELD_ARCHITECTURE.md is a
// transport-agnostic application boundary: Runtime realizes that boundary in
// code.
//
// Decoupling boundary:
//
//	cmd/lyra ──┐
//	           │ build
//	           ▼
//	    runtime.Runtime  ◄──── transport adapters
//	           ▲                 (HTTP, IPC, gRPC, MCP)
//	           │ owns
//	           ▼
//	    kernel + domain/*  (in-process implementations)
//
// Today the runtime and all transports live in the same Go process. The
// boundary still matters: transports depend on runtime, not on the concrete
// service constructors, so a future remote runtime implementation only needs
// to satisfy Runtime's accessor surface.
package runtime
