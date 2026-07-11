package runtime

import (
	"io"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Dependencies is the fully-assembled collaborator set a [Runtime] facade holds.
// The composition root (bootstrap) builds each collaborator and calls [New]. The
// single *kernel.Engine satisfies the facade's closer and every live-MCP port,
// so it is supplied once.
type Dependencies struct {
	Engine       *kernel.Engine
	Turns        turn.Dispatcher
	Conversation *conversation.Messages

	Sessions   sessionsvc.Store
	Interrupts interrupts.Store
	Transcript transcript.Store

	MCPRegistry mcpserver.Registry
	MCPPolicy   *atomic.Pointer[mcpserver.ToolPolicy]

	Titles   titleGenerator
	Codebase codebaseindex.Index

	Resources []io.Closer
}

// New builds a Runtime facade from already-assembled dependencies. It only
// wires; all construction/validation lives in the bootstrap ring's Assemble,
// which calls New.
func New(d Dependencies) *Runtime {
	return &Runtime{
		turns:              d.Turns,
		closer:             d.Engine,
		resources:          append([]io.Closer(nil), d.Resources...),
		history:            d.Conversation,
		sessions:           d.Sessions,
		interrupts:         d.Interrupts,
		transcript:         d.Transcript,
		mcpRegistry:        d.MCPRegistry,
		mcpLiveStatus:      d.Engine,
		mcpLiveTools:       d.Engine,
		mcpLiveConnections: d.Engine,
		mcpLiveRegistry:    d.Engine,
		mcpPolicy:          d.MCPPolicy,
		titles:             d.Titles,
		codebase:           d.Codebase,
	}
}
