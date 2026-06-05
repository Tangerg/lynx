// Package mcp bridges the Model Context Protocol (https://modelcontextprotocol.io/)
// into the lynx chat tool system in both directions.
//
// # Client side
//
// A Provider aggregates one or more *sdkmcp.ClientSession sources, fans
// listTools across them, and exposes the result as a list of chat.Tool. Each
// remote tool is wrapped in a Tool, which implements chat.Tool;
// the lynx ToolMiddleware can therefore drive an MCP tool exactly like any
// local tool. The cache is invalidated automatically when a server delivers
// a tools/list_changed notification, provided the caller wires
// (*Provider).OnToolListChanged into sdkmcp.ClientOptions.
//
// # Server side
//
// RegisterTools installs lynx chat.Tool implementations onto an
// *sdkmcp.Server using the low-level AddTool API. The handler converts a
// (string, error) result into the CallToolResult shape mandated by the
// protocol: a successful tool call yields a TextContent body; a failing
// tool yields IsError=true, never a JSON-RPC protocol error.
//
// # Naming
//
// The package shares its name with the official Go SDK
// (github.com/modelcontextprotocol/go-sdk/mcp). Consumers will normally
// import it as:
//
//	import (
//	    lynxmcp "github.com/Tangerg/lynx/mcp"
//	    "github.com/modelcontextprotocol/go-sdk/mcp"
//	)
//
// Inside this package the SDK is imported under the alias sdkmcp.
package mcp
