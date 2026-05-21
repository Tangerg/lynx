// Command mcpbridge shows the reverse direction: lynx exposes a
// chat.Tool as an MCP server so external hosts (Claude Desktop,
// Cursor, ...) can drive a lynx agent's tools.
//
// Run as a stdio MCP server (the host spawns the binary and talks
// over stdin/stdout):
//
//	go run ./agent/examples/mcpbridge
//
// The example uses an in-memory transport pair so it runs offline;
// for real deployments swap to mcp.StdioTransport{} (or a Streamable
// HTTP handler via mcp.NewStreamableHTTPHandler) — see lynx/mcp/DESIGN.md §5.
package main
