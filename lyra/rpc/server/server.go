// Package server realizes protocol.Runtime on top of Lyra's internal
// engine + service layer (API.md §0 model: Session → Run → Item). It's
// the single place where the JSON-RPC method table (rpc/dispatch) and
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
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
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
}

// Server is the Runtime implementation. Exposed via [New]; the returned
// interface is protocol.Runtime so callers can't reach past the typed
// surface.
type Server struct {
	rt         RuntimeServices
	serverInfo protocol.ServerInfo

	// runRegistry tracks live runs so CancelRun / ListRuns can find them
	// by id. Wired through chat.Service on the in-process path.
	runMu sync.Mutex
	runs  map[string]*runEntry

	// eventSeq is the server-wide monotonic source for RunEvent ids
	// (TRANSPORT.md §9.1). A single counter across all runs is strictly
	// stronger than the contract's per-root-stream requirement and lets
	// Last-Event-Id linearly resume / dedup even when the single SSE
	// connection interleaves events from more than one run.
	eventSeq atomic.Uint64
}

// nextEventID returns the next globally-monotonic RunEvent id, formatted
// evt_<zero-padded-decimal> (TRANSPORT.md §9.1, e.g. evt_00000000042).
// The fixed width keeps lexical and numeric ordering in agreement.
func (i *Server) nextEventID() string {
	return protocol.IDPrefixEvent + fmt.Sprintf("%011d", i.eventSeq.Add(1))
}

// runEntry holds bookkeeping for one in-flight run — used by CancelRun,
// ListRuns, and the event pump in runs.go.
type runEntry struct {
	runID       string
	sessionID   string
	turnID      string
	parentRunID string // set for continuation runs (runs.resume)
	cancel      context.CancelFunc
	hub         *runHub // per-run event fan-out + durable replay (streamable HTTP)
}

// New builds a Server. Returns an error when Runtime is nil.
func New(cfg Config) (protocol.Runtime, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("server: Runtime is required")
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "runtime"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0-dev"
	}
	return &Server{
		rt:         cfg.Runtime,
		serverInfo: cfg.ServerInfo,
		runs:       map[string]*runEntry{},
	}, nil
}

// Capabilities returns this Server's capability snapshot (API.md §9),
// delegating to the package-level [Capabilities] so the /v2/info
// sidecar can build the same snapshot without a constructed Server.
func (i *Server) Capabilities() protocol.ServerCapabilities {
	return Capabilities(i.rt)
}

// Capabilities builds the capability snapshot a Runtime advertises
// (API.md §9). It reflects actual wiring — features whose methods would
// return notImpl are advertised false, so the client never calls a
// method this build silently rejects.
func Capabilities(rt RuntimeServices) protocol.ServerCapabilities {
	memory := rt != nil && rt.Memory() != nil
	return protocol.ServerCapabilities{
		ProtocolVersion: protocol.ProtocolVersion,
		Events: []string{
			string(protocol.StreamRunStarted),
			string(protocol.StreamRunFinished),
			string(protocol.StreamItemStarted),
			string(protocol.StreamItemDelta),
			string(protocol.StreamItemCompleted),
		},
		// streamable-HTTP methods, machine-readable so the client knows which
		// calls return an event stream rather than hardcoding the names (§7/§9).
		StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe", "background.subscribe"},
		// Open features map (§9): advertise a new capability by adding a key.
		// Known keys absent here default to off on the client.
		Features: map[string]protocol.FeatureFlag{
			"reasoning": true,
			"mcp":       true,
			"memory":    memory,
			// Off until the corresponding engine support lands:
			"multimodal":    false,
			"checkpoints":   false,
			"background":    false,
			"subagents":     false,
			"skills":        false,
			"sessionExport": false,
			"relocate":      false,
			"clientTools":   false,
			"attachments":   protocol.AttachmentLimits{Enabled: false},
		},
		Providers: supportedProviderIDs(),
		Limits:    protocol.RuntimeLimits{MaxConcurrentRuns: 8},
	}
}

// supportedProviderIDs is the provider set this build can serve (E4 fix —
// was hardcoded empty, misreading as "no providers"). These are the
// provider TYPES the runtime supports; per-provider configured/key status
// is providers.list's job (apiKeyMasked), not the capability snapshot.
func supportedProviderIDs() []string {
	supported := config.SupportedProviders()
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
