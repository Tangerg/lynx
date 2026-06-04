package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	chatsvc "github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/service/session"
	toolsvc "github.com/Tangerg/lynx/lyra/internal/service/tool"
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
	Chat() chatsvc.Service
	Session() sessionsvc.Service
	Tool() toolsvc.Service
	Memory() memsvc.Service
	Approval() approval.Service
	Interrupts() interrupts.Store
	History() history.Store
	// ProviderInfo is the single configured provider id, model id, and
	// display-masked key (providers.list / models.list).
	ProviderInfo() (provider, model, apiKeyMasked string)
	// MCPServerNames lists the connected MCP servers (workspace.mcp.listServers).
	MCPServerNames() []string
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
}
