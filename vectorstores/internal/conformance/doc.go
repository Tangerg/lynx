// Package conformance contains the provider-independent contract tests for
// vector-store implementations.
//
// Every backend instantiates [Run] with a zero-value Store pointer. The suite
// intentionally exercises only operations that must finish during input
// validation, before a provider client, database connection, or embedding
// model can be used. Backend integration tests remain responsible for remote
// I/O and provider-specific query mapping.
package conformance
