// Package interaction defines the stable protocol exchanged between agent
// actions and framework-managed model/tool execution.
//
// The package contains data and narrow ports only. It does not run a process,
// call a model, or execute a tool. Concrete drivers such as toolloop implement
// the protocol, while runtime owns process identity, persistence, accounting,
// and event publication.
package interaction
