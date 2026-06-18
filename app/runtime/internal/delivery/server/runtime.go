package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// RuntimeServices is the accessor surface the protocol server needs from
// the runtime bundle. Defined here (consumer side) so the server depends
// on the narrow set of accessors it actually calls, not the concrete
// *internal/runtime.Runtime — which keeps the protocol layer free of an
// internal-package import and lets a future remote runtime (or a test
// fake) satisfy the surface without standing up the real bundle.
//
// *internal/runtime.Runtime satisfies this implicitly; the composition
// root (cmd/lyra) passes the concrete value where a RuntimeServices is
// expected.
type RuntimeServices interface {
	Chat() turn.Service
	Session() sessionsvc.Service
	Tool() toolsvc.Service
	Memory() knowledge.Service
	Approval() approval.Service
	Interrupts() interrupts.Store
	Transcript() transcript.Store
	// Providers is the provider registry — credentials + enablement that
	// providers.list / configure / test operate on (models.list reads the
	// catalog, not this).
	Providers() providersvc.Service
	// ProbeProvider validates a provider's credentials by building its
	// default-model client and issuing one minimal request — backs
	// providers.test. The runtime owns this because it owns client
	// construction; the protocol layer only maps the verdict to wire.
	ProbeProvider(ctx context.Context, entry providersvc.Provider) error
	// MCPServerStatuses lists every configured MCP server with its connection
	// state — connected and boot-failed alike (workspace.mcp.listServers).
	MCPServerStatuses() []kernel.McpServerStatus
	// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the
	// live tool set (workspace.mcp.reconnect). Returns engine.ErrUnknownMCPServer
	// for an unconfigured name.
	ReconnectMCPServer(ctx context.Context, name string) error
	// ListSkills enumerates the skills visible from cwd (project over global) —
	// backs workspace.listSkills. The engine owns skill sourcing + precedence.
	ListSkills(ctx context.Context, cwd string) ([]kernel.SkillInfo, error)
	// MCPTools lists tools per connected MCP server (server="" = all) —
	// backs workspace.mcp.listTools. The engine holds the dialed sessions.
	MCPTools(ctx context.Context, server string) ([]kernel.McpToolInfo, error)
	// DefaultModel is the runtime's configured default model — used to fill
	// Session.model for sessions that never explicitly selected one.
	DefaultModel() string
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
	// MessageCount / TruncateMessages back the chat-memory side of
	// sessions.rollback + fork{fromRunId}: record the per-run watermark and
	// truncate the message log to a kept boundary.
	MessageCount(ctx context.Context, sessionID string) (int, error)
	TruncateMessages(ctx context.Context, sessionID string, keepN int) error
}
