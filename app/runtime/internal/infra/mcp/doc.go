// Package mcp is the MCP-connection infra: it dials configured MCP servers,
// holds their live sessions, lists their tools, and reconnects them; it is the
// external-system adapter the engine builds its MCP tool set over. Pure infra
// (over the lynx mcp module + the go-sdk client); zero domain knowledge.
//
// A degraded boot is tolerated: a server that can't be reached is recorded
// "failed" and skipped, so one unreachable server never fails startup; only a
// config mistake (duplicate name / invalid entry) is fatal.
package mcp
