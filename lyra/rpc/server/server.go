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

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// Config bundles construction inputs.
type Config struct {
	// Runtime is the in-process runtime bundle. Required. Typed as the
	// narrow RuntimeServices accessor surface (the concrete
	// *internal/runtime.Runtime satisfies it).
	Runtime RuntimeServices

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "lyra-core", Version: "0.0.0-dev"} when zero.
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
}

// runEntry holds bookkeeping for one in-flight run — used by CancelRun,
// ListRuns, and the event pump in runs.go.
type runEntry struct {
	runID     string
	sessionID string
	turnID    string
	cancel    context.CancelFunc
}

// New builds a Server. Returns an error when Runtime is nil.
func New(cfg Config) (protocol.Runtime, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("server: Runtime is required")
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "lyra-core"
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
		Features: protocol.ServerFeatures{
			Reasoning: true,
			MCP:       true,
			Memory:    memory,
			// Off until the corresponding engine support lands:
			Multimodal:    false,
			Checkpoints:   false,
			Background:    false,
			Subagents:     false,
			Skills:        false,
			SessionExport: false,
			Relocate:      false,
			ClientTools:   false,
			Attachments:   protocol.AttachmentLimits{Enabled: false},
		},
		Providers: []string{},
		Limits:    protocol.RuntimeLimits{MaxConcurrentRuns: 8},
	}
}

// ─── helpers ────────────────────────────────────────────────────────

// notImpl marks a protocol method that exists in the contract but isn't
// backed on this build. Maps to capability_not_negotiated (API.md §8.2)
// — consistent with the feature flag advertised at initialize.
func notImpl(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
