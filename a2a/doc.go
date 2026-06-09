// Package a2a integrates the Agent-to-Agent (A2A) protocol into the lynx
// agent framework, wrapping the official SDK
// github.com/a2aproject/a2a-go/v2 ([sdka2a]/[a2asrv]/[a2aclient]).
//
// It has two sides, mirroring the mcp package:
//
//   - CLIENT — consume a remote A2A agent AS a lynx tool. [Dial] resolves a
//     remote AgentCard and opens a client; [NewAgentTool] wraps it as a
//     [chat.Tool] so a lynx agent can delegate to it through the ordinary
//     tool-calling loop. [DialAll] does both for a set of agents at once,
//     returning the []chat.Tool plus the clients to close on shutdown.
//
//   - SERVER — expose a lynx capability AS an A2A endpoint. Implement the
//     narrow [Agent] interface (text in, streamed text out); [NewExecutor]
//     adapts it to the SDK's [a2asrv.AgentExecutor] and [NewHTTPHandler]
//     mounts the JSON-RPC method endpoint plus the well-known AgentCard.
//
// The transport default is JSON-RPC over HTTP, matching the rest of the
// stack; the SDK's REST/gRPC bindings are not precluded but are not wired
// here.
//
// Naming convention: the SDK's core types package is imported as `sdka2a`
// to avoid colliding with this package's own name; the server and client
// SDK packages keep their names `a2asrv` / `a2aclient`.
package a2a
