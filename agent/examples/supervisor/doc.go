// Command supervisor demonstrates the "LLM-orchestrated multi-agent"
// pattern: a parent agent's LLM picks among sub-agents (each wrapped
// via runtime.AsChatTool) and chains them through toolloop.NewMiddleware.
//
// The parent's action body asks the LLM to brief a topic. The stub
// LLM:
//
//  1. first calls the "research-agent" sub-tool with {Title: ...} →
//     gets {Sources: [...]}
//  2. then calls the "summarize-agent" sub-tool with {Sources: [...]} →
//     gets {Summary: "..."}
//  3. finally emits the JSON Brief.
//
// chat.ToolMiddleware drives the LLM-tool loop; runtime.AsChatTool
// runs each sub-agent synchronously inside the parent process.
// Budget aggregation is automatic — the parent's Usage() sums the
// whole delegation tree.
//
// Run from repo root:
//
//	go run ./agent/examples/supervisor
package main
