// Command mcp-agent shows the complete client-side MCP integration
// in one file:
//
//   - mcp.Provider over an in-memory MCP server (one tool + one prompt)
//   - sampling.CreateMessageHandler wired to the platform's chat.Client
//   - tools/list_changed handler invalidating the Provider cache
//   - request-level metadata (process_id) forwarded via mcp.WithMeta
//   - action body that fetches a remote system prompt, then runs an
//     LLM tool loop where the LLM picks a tool exposed by the MCP server
//
// The example uses an in-memory transport pair so it runs offline;
// swap the transport for a CommandTransport / StreamableClientTransport
// in real deployments.
//
// Run from repo root:
//
//	go run ./agent/examples/mcp-agent
package main
