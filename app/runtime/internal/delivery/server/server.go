// Package server realizes protocol.Runtime on top of Lyra's internal
// kernel + domain layer (API.md §0 model: Session → Run → Item). It's
// the single place where the JSON-RPC method table (delivery/dispatch) and
// the runtime's chat / session / tool / memory services meet.
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

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	runstate "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
)

// Config bundles construction inputs.
type Config struct {
	// Runtime is the in-process runtime bundle. Required. Typed as the
	// narrow RuntimeServices accessor surface (the concrete
	// *internal/runtime.Runtime satisfies it).
	Runtime RuntimeServices

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "runtime", Version: "0.0.0-dev"} when zero — a vendor-neutral
	// name, since the protocol is consumed by arbitrary clients and the
	// rpc/protocol package is the codegen SSOT for other languages.
	ServerInfo protocol.ServerInfo

	// Workspace backs the git-backed VCS views and sessions.rollback's file
	// restore (restoreType). nil defaults to a disabled service —
	// features.{git,checkpoints} then reflect git availability.
	Workspace *workspace.Service
}

// Server is the Runtime implementation. Exposed via [New]; the returned
// interface is protocol.Runtime so callers can't reach past the typed
// surface.
type Server struct {
	rt         RuntimeServices
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

	// workspace owns the VCS views + per-session file checkpoints (snapshot at
	// each run boundary so sessions.rollback can restore files). Always
	// non-nil; checkpoints disabled (git unavailable) → features.checkpoints
	// reads false.
	workspace *workspace.Service
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
// can also reach server-owned background workers like RunScheduler.
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
	ws := cfg.Workspace
	if ws == nil {
		ws = workspace.New("") // disabled service: VCS reads still work, checkpoints off
	}
	return &Server{
		rt:         cfg.Runtime,
		serverInfo: cfg.ServerInfo,
		wsHub:      newWorkspaceHub(),
		workspace:  ws,
	}, nil
}

// coordinator returns the lifecycle coordinator for the cross-domain atomic
// write-sets (rollback truncation, session-delete cascade, import/restore,
// subtree purge). The handlers keep the wire decode + boundary decision + busy
// guards and delegate the multi-domain mutation here, so delivery stays a thin
// protocol layer. The Coordinator is stateless, so it's built on demand from rt
// — which keeps a bare &Server{rt: …} (tests) fully usable without a separate
// construction step.
func (s *Server) coordinator() *lifecycle.Coordinator { return lifecycle.New(s.rt) }

// Capabilities returns this Server's capability snapshot (API.md §9),
// delegating to the package-level [Capabilities] so the /v2/info
// sidecar can build the same snapshot without a constructed Server.
func (s *Server) Capabilities() protocol.ServerCapabilities {
	return Capabilities(s.rt)
}

// Capabilities builds the capability snapshot a Runtime advertises
// (API.md §9). It reflects actual wiring — features whose methods would
// return notImpl are advertised false, so the client never calls a
// method this build silently rejects.
func Capabilities(rt RuntimeServices) protocol.ServerCapabilities {
	memory := rt != nil && rt.Memory() != nil
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
		Providers: supportedProviderIDs(),
		Limits:    protocol.RuntimeLimits{MaxConcurrentRuns: 8},
	}
}

// supportedProviderIDs returns the provider types this build can serve.
// Called from [Capabilities] to advertise the runtime's provider support.
// Per-provider configured/key status is providers.list's job, not the
// capability snapshot.
func supportedProviderIDs() []string {
	supported := llm.SupportedProviders()
	out := make([]string, 0, len(supported))
	for _, p := range supported {
		out = append(out, string(p))
	}
	return out
}

// ─── helpers ────────────────────────────────────────────────────────

// notImpl marks a protocol method that exists in the contract but isn't
// backed on this build. Maps to capability_not_negotiated (API.md §8.2)
// — consistent with the feature flag advertised at initialize.
func notImpl(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
