// Package a2a integrates the Agent-to-Agent (A2A) protocol into the lynx
// agent framework, wrapping the official SDK
// github.com/a2aproject/a2a-go/v2 ([sdka2a]/[a2asrv]/[a2aclient]).
//
// It has two sides:
//
//   - CLIENT — [Tools] resolves remote AgentCards and wraps the resulting
//     agents as ordinary chat tools. It returns a close function for the
//     opened protocol clients.
//
//   - SERVER — expose a capability AS an A2A endpoint. Implement the
//     narrow [Agent] interface (text in, streamed text out); [NewHTTPHandler]
//     adapts it to the SDK and mounts the JSON-RPC method endpoint plus the
//     well-known AgentCard.
//
// The transport default is JSON-RPC over HTTP, matching the rest of the
// stack; the SDK's REST/gRPC bindings are not precluded but are not wired
// here.
//
// Naming convention: the SDK's core types package is imported as `sdka2a`
// to avoid colliding with this package's own name; the server and client
// SDK packages keep their names `a2asrv` / `a2aclient`.
package a2a
