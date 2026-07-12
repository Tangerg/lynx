// Package server realizes protocol.Runtime on top of Lyra's internal
// kernel + domain layer (API.md §0 model: Session → Run → Item). It's
// the single place where the JSON-RPC method table (delivery/dispatch) and
// the runtime's chat / session / tool / memory stores meet.
//
// Methods with an in-process equivalent (sessions, runs, items, tools,
// memory) are wired through; the rest return protocol.ErrCapabilityNotNeg,
// which the dispatch maps to capability_not_negotiated so the client
// sees an honest "off on this build" signal consistent with the
// capability flags advertised through discovery.
package server

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/capabilities"
	"github.com/Tangerg/lynx/app/runtime/internal/application/codebase"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// Config bundles construction inputs.
type Config struct {
	// Sessions is the application coordinator for the session/run lifecycle
	// write-sets and single-writer admission (rollback / delete cascade / fork /
	// restore / resume / working-tree gates). Required — the delivery layer drives
	// every lifecycle mutation through it.
	Sessions *sessions.Coordinator

	// Capabilities is the application coordinator for the runtime's capability +
	// configuration surface (tools / providers / model roles / default
	// provider+model / MCP). Required — the delivery settings + capability handlers
	// drive it directly.
	Capabilities *capabilities.Coordinator

	// Approvals is the application coordinator for the tool-permission stance +
	// approval rules. Required — the approval.* settings handlers drive it.
	Approvals *approvals.Coordinator

	// Models is the application coordinator for provider + model configuration
	// (providers.* / models.* / the default provider+model). Required — the
	// provider/model settings handlers + the capability snapshot drive it.
	Models *models.Coordinator

	// Coordinator owns the run lifecycle (admission / journal / pump / cancel),
	// built + owned by the composition root (bootstrap.Host). Required — delivery
	// drives it as a use-case surface but never constructs or closes it (§11.1).
	Coordinator *runs.Coordinator

	// Queries is the application read coordinator for a session's durable
	// execution record (transcript / history / interrupts). Required — the
	// items.list / messages.list / interrupts.list handlers drive it.
	Queries *queries.Coordinator

	// TurnControl is the turn-start adapter (plan / start / steer a turn). Required
	// — the runs.start / runs.steer handlers drive it. It speaks the agent-SDK turn
	// types, so it lives in the adapter ring, not application.
	TurnControl *turn.Control

	// FileChanges is the composition-root bridge the run pump publishes live
	// file-change nudges through; the Server installs a consumer that maps them to
	// wire workspace events on the hub. Required in production; nil in tests that
	// don't exercise the workspace stream.
	FileChanges FileChangeSource

	// MCPStatus is the composition-root bridge the capabilities coordinator
	// publishes MCP connection transitions through; the Server installs a consumer
	// that maps them to mcp.serverChanged workspace events. Required in production;
	// nil in tests that don't exercise the MCP status stream.
	MCPStatus MCPStatusSource

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "runtime", Version: "0.0.0-dev"} when zero — a vendor-neutral
	// name, since the protocol is consumed by arbitrary clients and the
	// rpc/protocol package is the codegen SSOT for other languages.
	ServerInfo protocol.ServerInfo

	// Schedules is the application coordinator for cron-triggered headless runs
	// (schedules.* + the background worker). nil defaults to a disabled
	// coordinator, so a build without scheduling reports capability_not_negotiated.
	Schedules *schedules.Coordinator

	// Workspace is the application coordinator for the project developer surface
	// (memory / skills / recipes / hooks). nil defaults to a disabled coordinator
	// (every dependency nil), so those workspace.* methods degrade gracefully.
	Workspace *workspaceapp.Coordinator

	// Codebase is the application coordinator for the @codebase semantic index
	// (codebase.search / status / reindex). Required — a nil-index coordinator
	// still reports "unavailable" gracefully, so the handlers always have one.
	Codebase *codebase.Coordinator
}

// Server is the protocol.Runtime implementation exposed via [New].
type Server struct {
	serverInfo protocol.ServerInfo

	// sessions owns the session/run lifecycle write-sets and single-writer
	// admission gates (rollback / delete cascade / fork / restore / resume /
	// working-tree). Injected by the composition root; never nil after New.
	sessions *sessions.Coordinator

	// capabilities owns the runtime capability + configuration use cases (tools /
	// providers / model roles / defaults / MCP). Injected by the composition root;
	// never nil after New.
	capabilities *capabilities.Coordinator

	// approvals owns the tool-permission stance + approval-rule use cases. Injected
	// by the composition root; never nil after New.
	approvals *approvals.Coordinator

	// models owns provider + model configuration (registry / catalog / roles /
	// defaults). Injected by the composition root; never nil after New.
	models *models.Coordinator

	// codebase owns the @codebase semantic-index use cases (search / status /
	// reindex). Injected by the composition root; never nil after New.
	codebase *codebase.Coordinator

	// coordinator owns the run lifecycle — admission, the per-run event Journal,
	// the segment pumps, cancel. Built + owned by the composition root
	// (bootstrap.Host); delivery drives it as a use-case surface and never closes
	// it (§11.1). Injected by New; never nil after New.
	coordinator *runs.Coordinator

	// queries is the application read coordinator for a session's durable
	// execution record (transcript / history / interrupts). Injected by New.
	queries *queries.Coordinator

	// turnControl is the turn-start adapter (plan / start / steer). Injected by New.
	turnControl *turn.Control

	// schedules owns the cron-triggered headless-run use cases (schedules.* + the
	// background worker), injected by the composition root. Never nil after New.
	schedules *schedules.Coordinator

	// workspace owns the project developer-surface use cases (memory / skills /
	// recipes / hooks), injected by the composition root. Never nil after New.
	workspace *workspaceapp.Coordinator

	// wsHub fans non-run workspace events (files/skills/mcp changes) out to
	// workspace.subscribe streams (AUX_API §3). Ephemeral, lossy, connection-
	// scoped — distinct from the durable per-run hubs.
	wsHub *workspaceHub

	// closed gates new workspace subscriptions once the Server is shutting down.
	// Each stream's own lifetime is its request ctx (canceled on client disconnect
	// or the transport's forced shutdown); delivery owns no task group (§16 rule 5).
	closed atomic.Bool
}

// FileChangeSource is the delivery-side view of the composition-root file-change
// bridge: the Server installs a consumer (Observe) that maps the run pump's live
// file-change nudges to wire workspace events on the hub. The concrete notifier
// is owned by the Host, which also passes its publish side to the run effects.
type FileChangeSource interface {
	Observe(sink func(cwd string, paths []string))
}

// MCPStatusSource is the delivery-side view of the composition-root MCP-status
// bridge: the Server installs a consumer (Observe) that maps the capabilities
// coordinator's connection transitions to mcp.serverChanged workspace events. The
// concrete notifier is owned by the Host, which passes its publish side to the
// capabilities coordinator.
type MCPStatusSource interface {
	Observe(sink func(ctx context.Context, server string, connecting bool))
}

// Close marks the Server shut down so new workspace subscriptions are rejected;
// in-flight streams end with their request contexts, and the run coordinator's
// pumps are joined by the Host, not here (§11.1). Safe to call repeatedly.
func (s *Server) Close() {
	if s == nil {
		return
	}
	s.closed.Store(true)
}

// runCleanupTimeout bounds the request-detached work that drives a parked run's
// durable cancel, so a stuck store can't wedge cancellation.
const runCleanupTimeout = 5 * time.Second

// New builds a Server. Returns an error when a required coordinator is nil. The
// concrete *Server is returned (it satisfies [protocol.Runtime]) so the
// composition root can also reach process entry points like RunScheduler.
func New(cfg Config) (*Server, error) {
	if cfg.Sessions == nil {
		return nil, errors.New("server: Sessions is required")
	}
	if cfg.Capabilities == nil {
		return nil, errors.New("server: Capabilities is required")
	}
	if cfg.Approvals == nil {
		return nil, errors.New("server: Approvals is required")
	}
	if cfg.Models == nil {
		return nil, errors.New("server: Models is required")
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "runtime"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0-dev"
	}
	if cfg.Coordinator == nil {
		return nil, errors.New("server: Coordinator is required")
	}
	if cfg.Queries == nil {
		return nil, errors.New("server: Queries is required")
	}
	if cfg.TurnControl == nil {
		return nil, errors.New("server: TurnControl is required")
	}
	scheduleCoord := cfg.Schedules
	if scheduleCoord == nil {
		scheduleCoord = schedules.NewCoordinator(nil, nil) // disabled: schedules.* report capability_not_negotiated
	}
	workspaceCoord := cfg.Workspace
	if workspaceCoord == nil {
		workspaceCoord = workspaceapp.New(workspaceapp.Config{}) // disabled: memory/skills/recipes/hooks all no-op
	}
	codebaseCoord := cfg.Codebase
	if codebaseCoord == nil {
		codebaseCoord = codebase.New(nil) // disabled: codebase.* report unavailable
	}
	srv := &Server{
		sessions:     cfg.Sessions,
		capabilities: cfg.Capabilities,
		approvals:    cfg.Approvals,
		models:       cfg.Models,
		codebase:     codebaseCoord,
		coordinator:  cfg.Coordinator,
		queries:      cfg.Queries,
		turnControl:  cfg.TurnControl,
		serverInfo:   cfg.ServerInfo,
		wsHub:        newWorkspaceHub(),
		schedules:    scheduleCoord,
		workspace:    workspaceCoord,
	}
	// The run pump publishes live file-change nudges through the composition-root
	// bridge; the Server maps each to a wire workspace event on its hub. This is
	// the seam that lets the coordinator be built in the Host (§11.1/§13.2) — its
	// effects need a publish sink, but the hub is constructed here in delivery.
	if cfg.FileChanges != nil {
		srv.wsHub.observe(cfg.FileChanges)
	}
	// MCP reconnect/authorize run fire-and-forget in the capabilities coordinator;
	// their connecting → settled transitions reach the workspace hub through this
	// bridge, mapped to mcp.serverChanged frames.
	if cfg.MCPStatus != nil {
		srv.observeMCPStatus(cfg.MCPStatus)
	}
	return srv, nil
}

// Capabilities returns this Server's capability snapshot (API.md §9),
// delegating to the package-level [Capabilities] so the /v2/info
// sidecar can build the same snapshot without a constructed Server.
func (s *Server) Capabilities() protocol.ServerCapabilities {
	return Capabilities(s.models, s.workspace.HasMemory())
}

// capabilityAccess is the slice of the models coordinator the capability snapshot
// needs; the coordinator (and any test fake of it) satisfies it.
type capabilityAccess interface {
	SupportedProviders() []providersvc.Metadata
}

// Capabilities builds the capability snapshot a Runtime advertises
// (API.md §9). It reflects actual wiring — features whose methods would
// return capability_not_negotiated are advertised false, so the client never calls a
// method this build silently rejects. hasMemory comes from the workspace
// coordinator (the long-term knowledge store may be absent).
func Capabilities(rt capabilityAccess, hasMemory bool) protocol.ServerCapabilities {
	memory := hasMemory
	return protocol.ServerCapabilities{
		ProtocolVersion: protocol.ProtocolVersion,
		Events: []protocol.StreamEventType{
			protocol.StreamRunStarted,
			protocol.StreamRunProgress,
			protocol.StreamRunFinished,
			protocol.StreamItemStarted,
			protocol.StreamItemDelta,
			protocol.StreamItemCompleted,
			protocol.StreamStateSnapshot,
			protocol.StreamStateDelta,
			protocol.StreamCustom,
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
			"todos":       true, // state.snapshot{todos} from the todo_write tool
			"compaction":  true, // compaction Item boundaries
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
// — consistent with the feature flag advertised through discovery.
func capabilityNotNegotiated(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
