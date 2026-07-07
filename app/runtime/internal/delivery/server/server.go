// Package server realizes protocol.Runtime on top of Lyra's internal
// kernel + domain layer (API.md §0 model: Session → Run → Item). It's
// the single place where the JSON-RPC method table (delivery/dispatch) and
// the runtime's chat / session / tool / memory stores meet.
//
// Methods with an in-process equivalent (sessions, runs, items, tools,
// memory) are wired through; the rest return protocol.ErrCapabilityNotNeg,
// which the dispatch maps to capability_not_negotiated so the client
// sees an honest "off on this build" signal consistent with the
// capability flags advertised at initialize.
package server

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	runstate "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// Config bundles construction inputs.
type Config struct {
	// Runtime is the in-process runtime bundle. Required. Typed as the
	// narrow RuntimePort (the concrete *internal/runtime.Runtime satisfies it).
	Runtime RuntimePort

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "runtime", Version: "0.0.0-dev"} when zero — a vendor-neutral
	// name, since the protocol is consumed by arbitrary clients and the
	// rpc/protocol package is the codegen SSOT for other languages.
	ServerInfo protocol.ServerInfo

	// Checkpoints backs run-boundary snapshots and sessions.rollback file
	// restore (restoreType). nil defaults to a disabled checkpoint adapter.
	Checkpoints *workspace.Checkpoints
}

// Server is the Runtime implementation. Exposed via [New]; the returned
// interface is protocol.Runtime so callers can't reach past the typed
// surface.
type Server struct {
	runtimeBindings
	serverInfo protocol.ServerInfo

	// runs tracks active run segments and admission claims. The domain registry
	// owns the single-writer-per-session invariant; delivery supplies only the
	// in-process resources needed to stream and cancel live runs.
	runs runstate.Registry[*runHandle]

	// eventSeq is the server-wide monotonic source for RunEvent ids
	// (TRANSPORT.md §9.1). A single counter across all runs is strictly
	// stronger than the contract's per-root-stream requirement and lets
	// Last-Event-Id linearly resume / dedup even when the single SSE
	// connection interleaves events from more than one run.
	eventSeq atomic.Uint64

	// wsHub fans non-run workspace events (files/skills/mcp changes) out to
	// workspace.subscribe streams (AUX_API §3). Ephemeral, lossy, connection-
	// scoped — distinct from the durable per-run hubs.
	wsHub *workspaceHub

	// checkpoints owns per-session file snapshots: snapshot at each run boundary
	// so sessions.rollback can restore files. VCS reads stay stateless package
	// functions in adapter/workspace.
	checkpoints *workspace.Checkpoints
}

// nextEventID returns the next globally-monotonic RunEvent id, formatted
// evt_<zero-padded-decimal> (TRANSPORT.md §9.1, e.g. evt_00000000042).
// The fixed width keeps lexical and numeric ordering in agreement.
func (s *Server) nextEventID() string {
	return protocol.IDPrefixEvent + fmt.Sprintf("%011d", s.eventSeq.Add(1))
}

// runHandle holds delivery-owned resources for one in-flight run segment.
type runHandle struct {
	cancel context.CancelFunc
	hub    *runHub
}

// New builds a Server. Returns an error when Runtime is nil. The concrete
// *Server is returned (it satisfies [protocol.Runtime]) so the composition root
// can also reach process entry points like RunScheduler.
func New(cfg Config) (*Server, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("server: Runtime is required")
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "runtime"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0-dev"
	}
	checkpoints := cfg.Checkpoints
	if checkpoints == nil {
		checkpoints = workspace.NewCheckpoints("") // disabled: VCS reads still work, checkpoints off
	}
	return &Server{
		runtimeBindings: bindRuntime(cfg.Runtime),
		serverInfo:      cfg.ServerInfo,
		wsHub:           newWorkspaceHub(),
		checkpoints:     checkpoints,
	}, nil
}

// Capabilities returns this Server's capability snapshot (API.md §9),
// delegating to the package-level [Capabilities] so the /v2/info
// sidecar can build the same snapshot without a constructed Server.
func (s *Server) Capabilities() protocol.ServerCapabilities {
	return Capabilities(s.runtimeBindings)
}

type capabilityAccess interface {
	HasMemory() bool
	SupportedProviders() []providersvc.Metadata
}

func (b runtimeBindings) HasMemory() bool {
	return b.memoryAvailability != nil && b.memoryAvailability.HasMemory()
}

func (b runtimeBindings) SupportedProviders() []providersvc.Metadata {
	if b.providerCatalog == nil {
		return nil
	}
	return b.providerCatalog.SupportedProviders()
}

type runtimeBindings struct {
	turn               turnAccess
	sessionCatalog     sessionCatalogAccess
	sessionMutations   sessionMutationAccess
	sessionDefaults    sessionDefaultModelAccess
	transcriptContent  transcriptContentAccess
	transcriptRuns     transcriptRunAccess
	lifecycle          lifecycleAccess
	runSegments        runSegmentAccess
	history            historyAccess
	interrupts         interruptQueryAccess
	toolCatalog        toolCatalogAccess
	toolInvocations    toolInvocationAccess
	memoryAvailability memoryAvailabilityAccess
	memoryStore        memoryStoreAccess
	approvalModes      approvalModeAccess
	approvalRules      approvalRuleAccess
	scheduleCatalog    scheduleCatalogAccess
	scheduleMutations  scheduleMutationAccess
	scheduleRuns       scheduleRunRecorderAccess
	scheduleWorker     scheduleWorkerAccess
	providerRegistry   providerRegistryAccess
	providerCatalog    providerCatalogAccess
	providerDefaults   providerDefaultAccess
	mcpStatus          mcpStatusAccess
	mcpTools           mcpToolCatalogAccess
	mcpConnections     mcpConnectionAccess
	mcpRegistry        mcpRegistryAccess
	workspaceCatalog   workspaceCatalogAccess
	hooks              hookAccess
	modelRoles         modelRoleAccess
	codebase           codebaseAccess
	maintenance        maintenanceAccess
}

func bindRuntime(rt RuntimePort) runtimeBindings {
	return runtimeBindings{
		turn:               rt,
		sessionCatalog:     rt,
		sessionMutations:   rt,
		sessionDefaults:    rt,
		transcriptContent:  rt,
		transcriptRuns:     rt,
		lifecycle:          rt,
		runSegments:        rt,
		history:            rt,
		interrupts:         rt,
		toolCatalog:        rt,
		toolInvocations:    rt,
		memoryAvailability: rt,
		memoryStore:        rt,
		approvalModes:      rt,
		approvalRules:      rt,
		scheduleCatalog:    rt,
		scheduleMutations:  rt,
		scheduleRuns:       rt,
		scheduleWorker:     rt,
		providerRegistry:   rt,
		providerCatalog:    rt,
		providerDefaults:   rt,
		mcpStatus:          rt,
		mcpTools:           rt,
		mcpConnections:     rt,
		mcpRegistry:        rt,
		workspaceCatalog:   rt,
		hooks:              rt,
		modelRoles:         rt,
		codebase:           rt,
		maintenance:        rt,
	}
}

// Capabilities builds the capability snapshot a Runtime advertises
// (API.md §9). It reflects actual wiring — features whose methods would
// return capability_not_negotiated are advertised false, so the client never calls a
// method this build silently rejects.
func Capabilities(rt capabilityAccess) protocol.ServerCapabilities {
	memory := rt != nil && rt.HasMemory()
	return protocol.ServerCapabilities{
		ProtocolVersion: protocol.ProtocolVersion,
		Events: []protocol.StreamEventType{
			protocol.StreamRunStarted,
			protocol.StreamRunProgress,
			protocol.StreamRunFinished,
			protocol.StreamItemStarted,
			protocol.StreamItemDelta,
			protocol.StreamItemCompleted,
		},
		// streamable-HTTP methods, machine-readable so the client knows which
		// calls return an event stream rather than hardcoding the names (§7/§9).
		StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe", "workspace.subscribe"},
		// Open features map (§9): advertise a new capability by adding a key.
		// Known keys absent here default to off on the client.
		Features: map[string]protocol.FeatureFlag{
			"reasoning": true,
			"mcp":       true,
			"memory":    memory,
			"skills":    true,                     // workspace.listSkills (project + global enumeration)
			"git":       workspace.GitAvailable(), // workspace.listFileChanges / getDiff (git binary on PATH)
			"fileWatch": true,                     // workspace.subscribe watches → files.changed (fsnotify)
			"lsp":       true,                     // code-intelligence tools (definition/refs/hover/symbols/diagnostics) + auto type-check on edit

			"sessionExport": true, // sessions.export (inline json/md) + sessions.import (restore)
			// File checkpoints (restoreType on rollback) ride the shadow-git
			// store, which needs the git binary — same gate as the git feature.
			"checkpoints": workspace.GitAvailable(),
			"multimodal":  true, // image input: runs.start input image blocks (Mime + base64 Data)
			"relocate":    true, // sessions.update cwd-relocate
			// Off until the corresponding engine support lands:
			"subagents":   false,
			"clientTools": false,
		},
		Providers: providerIDs(rt.SupportedProviders()),
		Limits:    protocol.RuntimeLimits{MaxConcurrentRuns: 8},
	}
}

func providerIDs(supported []providersvc.Metadata) []string {
	out := make([]string, 0, len(supported))
	for _, meta := range supported {
		out = append(out, meta.ID)
	}
	return out
}

// ─── helpers ────────────────────────────────────────────────────────

// capabilityNotNegotiated marks a protocol method that exists in the contract
// but isn't backed on this build. Maps to capability_not_negotiated (API.md §8.2)
// — consistent with the feature flag advertised at initialize.
func capabilityNotNegotiated(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
