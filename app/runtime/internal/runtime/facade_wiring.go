package runtime

import (
	"io"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Dependencies is the fully-assembled collaborator set a [Runtime] facade holds.
// The composition root (bootstrap) builds each collaborator and calls [New]; the
// in-package [Assemble] is the convenience that wires them from a [Config]. The
// single *kernel.Engine satisfies the facade's closer, skill catalog, and every
// live-MCP port, so it is supplied once.
type Dependencies struct {
	Engine       *kernel.Engine
	Turns        turn.Dispatcher
	Tools        toolsvc.Registry
	Approval     approval.Policy
	Conversation *conversation.Messages
	Resolver     chatClientResolver

	Sessions   sessionsvc.Store
	Interrupts interrupts.Store
	Transcript transcript.Store
	Memory     knowledge.Store
	Providers  provider.Registry

	MCPRegistry mcpserver.Registry
	MCPPolicy   *atomic.Pointer[mcpserver.ToolPolicy]

	DefaultProvider string
	DefaultModel    string

	Titles       titleGenerator
	UtilityCell  *atomic.Pointer[modelrole.Role]
	UtilityStore utilityRoleSaver

	HookInspection   hookInspector
	HookTrust        HookTrustStore
	RecipesGlobalDir string

	EmbeddingCell  *atomic.Pointer[modelrole.Role]
	Embeddings     *modelclient.EmbeddingResolver
	EmbeddingStore embeddingRoleSaver
	Codebase       codebaseindex.Index

	Transactor Transactor
	Resources  []io.Closer
}

// New builds a Runtime facade from already-assembled dependencies. It only
// wires; all construction/validation lives in [Assemble] (and, in the target
// architecture, in the bootstrap ring that will call New directly).
func New(d Dependencies) *Runtime {
	return &Runtime{
		turns:              d.Turns,
		closer:             d.Engine,
		resources:          append([]io.Closer(nil), d.Resources...),
		skillCatalog:       d.Engine,
		tools:              d.Tools,
		memory:             d.Memory,
		approval:           d.Approval,
		history:            d.Conversation,
		sessions:           d.Sessions,
		interrupts:         d.Interrupts,
		transcript:         d.Transcript,
		providers:          d.Providers,
		mcpRegistry:        d.MCPRegistry,
		mcpLiveStatus:      d.Engine,
		mcpLiveTools:       d.Engine,
		mcpLiveConnections: d.Engine,
		mcpLiveRegistry:    d.Engine,
		mcpPolicy:          d.MCPPolicy,
		defaultProvider:    d.DefaultProvider,
		defaultModel:       d.DefaultModel,
		titles:             d.Titles,
		utility:            d.UtilityCell,
		utilityClients:     d.Resolver,
		utilStore:          d.UtilityStore,
		hookInspection:     d.HookInspection,
		hookTrust:          d.HookTrust,
		recipesGlobalDir:   d.RecipesGlobalDir,
		embeddingCell:      d.EmbeddingCell,
		embeddings:         d.Embeddings,
		embeddingStore:     d.EmbeddingStore,
		codebase:           d.Codebase,
		transactor:         d.Transactor,
	}
}
