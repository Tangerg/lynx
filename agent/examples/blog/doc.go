// Command blog is a richer example that exercises the planner:
// three actions (research, outline, write) chained by data
// dependencies, one terminal goal. No LLM is needed — each action
// returns a stub artifact — but the GOAP planner has to figure out
// the topological order on its own.
//
// Run from repo root:
//
//	go run ./agent/examples/blog
package main
