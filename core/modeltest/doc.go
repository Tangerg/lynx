// Package modeltest provides reusable contract tests for implementations of
// Core model interfaces. Provider modules use these suites and transport
// fixtures from their tests; production code should not import this package.
//
// A provider that writes its own tests proves only that it works, not that it
// works the same way as its siblings. These suites exist so that agreement is
// the default: request immutability, delta validity, terminal-error position,
// cancellation identity, and teardown are asserted identically for every
// provider, and a provider that quietly diverges fails here rather than in a
// caller that swapped one vendor for another.
//
// The suites are transport-honest. They drive a real SDK against a local
// server instead of a hand-written fake, because the failures worth catching —
// a wrong path, a mis-encoded option, a stream that never closes — live in the
// SDK layer that a fake would replace.
package modeltest
