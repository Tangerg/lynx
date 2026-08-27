// Package mcp provides scope helpers around the Model Context Protocol
// (https://modelcontextprotocol.io/).
//
// Use the official Go SDK package (github.com/modelcontextprotocol/go-sdk/mcp)
// for protocol clients, servers, sessions, and transports. The root scope
// package keeps the small adapters scope needs around those SDK primitives:
// context metadata, reverse-capability helpers, tool.Tool wrapping, tool
// registration and prompt conversion.
//
// # Naming
//
// The package shares its name with the official Go SDK
// (github.com/modelcontextprotocol/go-sdk/mcp). Consumers will normally
// import it as:
//
//	import (
//	    scopemcp "github.com/Tangerg/scope/mcp"
//	    sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
//	)
//
// Inside this package the SDK is imported under the alias sdkmcp.
package mcp
