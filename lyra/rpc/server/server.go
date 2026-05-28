// Package server realises protocol.Runtime on top of Lyra's
// internal engine + service layer. It's the single place where the
// JSON-RPC method table (rpc/dispatch) and the runtime's existing
// chat / session / approval / tool services meet.
//
// Methods with an in-process equivalent (sessions, runs, approvals,
// some tool reads) are wired through; the rest return protocol.ErrNotImplemented,
// which the dispatch maps to JSON-RPC -32601 method not found so
// the client sees an honest "not supported on this build" signal.
package server

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ProtocolVersion is the wire version this build implements (API.md
// §8.1: date string).
const ProtocolVersion = "2026-05-28"

// Config bundles construction inputs.
type Config struct {
	// Runtime is the in-process runtime bundle. Required.
	Runtime *lyraruntime.Runtime

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "lyra-core", Version: "0.0.0-dev"} when zero.
	ServerInfo protocol.ServerInfo
}

// Server is the Runtime implementation. Exposed via [New] but the
// returned interface is protocol.Runtime — keeps callers from
// reaching past the typed surface.
type Server struct {
	rt         *lyraruntime.Runtime
	serverInfo protocol.ServerInfo

	// runRegistry tracks live runs so CancelRun can find them by id.
	// Wired through chat.Service.Cancel on the in-process path.
	runMu sync.Mutex
	runs  map[string]*runEntry
}

// runEntry holds the bookkeeping for one in-flight run — used by
// CancelRun + the event pump in runs.go.
type runEntry struct {
	runID     string
	sessionID string
	turnID    string
	cancel    context.CancelFunc
}

// New builds an Server. Returns an error when Runtime is nil.
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

// ServerCapabilities returns the static capability snapshot this
// build advertises. Pure function so the HTTP /v1/info sidecar can
// reach it without needing a Runtime instance.
func Capabilities() protocol.ServerCapabilities {
	return protocol.ServerCapabilities{
		Events: protocol.EventCapabilities{
			Standard: []string{
				"RUN_STARTED", "RUN_FINISHED", "RUN_ERROR",
				"STEP_STARTED", "STEP_FINISHED",
				"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END",
				"TOOL_CALL_START", "TOOL_CALL_ARGS", "TOOL_CALL_END", "TOOL_CALL_RESULT",
				"REASONING_MESSAGE_START", "REASONING_MESSAGE_CONTENT", "REASONING_MESSAGE_END",
				"CUSTOM", "RAW",
			},
			Custom: []string{
				"plan_generated",
				"tool_call_approval",
			},
		},
		Features: protocol.ServerFeatureCapabilities{
			Reasoning:     true,
			MCP:           true,
			Multimodal:    false,
			Checkpoints:   false,
			Interrupts:    false,
			Background:    false,
			Subagents:     false,
			Skills:        false,
			SessionExport: false,
			Attachments: protocol.AttachmentLimits{
				Enabled: false,
			},
		},
		Providers: []string{}, // future: derive from chat.Client metadata
		Limits:    protocol.Limits{MaxConcurrentRuns: 8},
	}
}

// ─── helpers ────────────────────────────────────────────────────────

// genID is the canonical id generator. UUID v7 (sortable timestamp +
// random tail) — API.md §6.3 expects v7 so receivers can order ids
// without consulting timestamps. Falls back to v4 only when the v7
// RNG fails (effectively never).
func genID() string {
	u, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return u.String()
}

// notImpl is a tiny helper so each stubbed method reads as one line.
func notImpl(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrNotImplemented, method)
}
