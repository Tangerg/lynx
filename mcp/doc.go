// Package mcp provides lynx helpers around the Model Context Protocol
// (https://modelcontextprotocol.io/).
//
// Use the official Go SDK package (github.com/modelcontextprotocol/go-sdk/mcp)
// for protocol clients, servers, sessions, and transports. The root lynx
// package keeps the small adapters lynx needs around those SDK primitives:
// context metadata, reverse-capability helpers, chat.Tool wrapping, tool
// registration, sampling, and prompt conversion.
//
// # Naming
//
// The package shares its name with the official Go SDK
// (github.com/modelcontextprotocol/go-sdk/mcp). Consumers will normally
// import it as:
//
//	import (
//	    lynxmcp "github.com/Tangerg/lynx/mcp"
//	    sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
//	)
//
// Inside this package the SDK is imported under the alias sdkmcp.
package mcp
