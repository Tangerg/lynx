// Package examples is the overview for Scope's runnable demonstrations. It
// declares no API; each subdirectory is a main package showing how several
// modules compose.
//
// Nothing in the repository depends on this module, and it is not published for
// consumption. Example code may import any Scope module; no Scope module may
// import an example. That one-way direction is what keeps a demonstration from
// quietly becoming an API nobody agreed to support.
//
// A capability a real caller would want does not belong here — it belongs in
// tools or its owning domain module, where it gets a contract and tests. An
// example that needs a provider key reads it from the environment and says so
// when it is missing, rather than failing with a transport error.
//
// See README.md for how to run one and ARCHITECTURE.md for the rules this
// rests on.
package examples
