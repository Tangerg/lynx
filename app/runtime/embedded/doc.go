// Package embedded lets a Go program own a complete Lyra Runtime in-process.
// It exposes the same typed protocol operations as the HTTP binding without a
// listener, authentication token, JSON-RPC envelope or SSE framing. Operation
// errors support errors.Is against protocol sentinels and errors.As to
// protocol.ProblemError.
package embedded
