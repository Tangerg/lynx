package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// Dispatcher routes inbound JSON-RPC messages to typed Runtime
// methods. One instance per connection — it carries the per-conn
// handshake state (initialized + negotiated capabilities) which the
// HTTP transport keys by request affinity (cookie / sticky token) or
// the InProcess transport sees as a single long-lived value.
type Dispatcher struct {
	api protocol.Runtime

	// initialized flips once runtime.initialize succeeds. Until then
	// every business method returns -32011 protocol_violation. atomic
	// so concurrent dispatch goroutines see a consistent view.
	initialized atomic.Bool
}

// New builds a Dispatcher bound to the given Runtime. Concurrency:
// the returned Dispatcher is safe for parallel Handle calls; per-conn
// state lives on it, so use one Dispatcher per logical session.
func New(api protocol.Runtime) *Dispatcher {
	return &Dispatcher{api: api}
}

// HandleResult holds what the dispatcher returns after processing
// one inbound message. Response is non-nil for Request inputs;
// EventStream / RunID are populated for streaming methods (runs.start,
// workspace.terminal.subscribe, background.subscribe) so the transport
// can pipe events as notifications.
type HandleResult struct {
	// Response is the synchronous JSON-RPC reply. nil when the input
	// was a notification (no id, no response on the wire).
	Response *transport.Response

	// RunID is set when the dispatched method initiated a stream — the
	// transport uses it as the routing key for notifications/run/event
	// (API.md v4 §3.1: the resource id IS the stream identifier; no
	// separate streamHandle).
	RunID string

	// EventStream is the channel of AG-UI events for streaming
	// methods. Closed when the underlying run ends.
	EventStream <-chan protocol.AgUiEvent

	// ResultStream yields one RunResult when a run ends, then closes.
	// Transports read it after EventStream closes to build
	// notifications/run/closed (API.md §3.1 / §6.3). nil for streaming
	// methods that have no terminal RunResult (terminal/background).
	ResultStream <-chan protocol.RunResult
}

// Handle is the entry point — every inbound transport.Message goes
// through here. The transport layer handles framing and observability
// before calling Handle.
//
// ExpectedMethod, when non-empty, is the method name the transport
// extracted from the URL path (HTTP /v1/rpc/{method}); the dispatcher
// cross-checks it against the body method and returns -32011 on
// mismatch.
func (d *Dispatcher) Handle(ctx context.Context, msg transport.Message, expectedMethod string) HandleResult {
	// Discriminate Request vs Response. Responses arriving at the
	// dispatcher are invalid (we don't issue outbound requests today)
	// and get rejected as a malformed envelope. Nil msg also falls
	// through here.
	req, ok := msg.(*transport.Request)
	if !ok || req == nil {
		return responseError(transport.ID{}, transport.NewError(transport.CodeInvalidRequest, nil))
	}

	if expectedMethod != "" && expectedMethod != req.Method {
		return responseError(req.ID, protocolViolation(fmt.Sprintf(
			"url method %q does not match body method %q", expectedMethod, req.Method,
		)))
	}

	// API.md v4 §1.1: id MUST be a JSON number. Reject string ids
	// (and any other non-number type) at the dispatcher boundary.
	// IDs come from MakeID — nil, int64, or string; we only allow
	// int64 (or absent, for Notifications).
	if req.ID.IsValid() {
		if _, ok := req.ID.Raw().(int64); !ok {
			return responseError(req.ID, transport.NewError(transport.CodeInvalidRequest, problemDataFrom(
				fmt.Errorf("id must be a JSON number, got %T", req.ID.Raw()),
			)))
		}
	}

	// Notifications: no response. We still dispatch so cancel-style
	// notifications take effect; errors are dropped silently per
	// JSON-RPC spec (logged elsewhere).
	if !req.IsCall() {
		d.handleNotification(ctx, req)
		return HandleResult{}
	}

	// Gate business methods behind initialize.
	if !d.initialized.Load() && req.Method != MethodInitialize && req.Method != MethodPing {
		return responseError(req.ID, protocolViolation(
			"runtime.initialize must succeed before any business method",
		))
	}

	return d.dispatchRequest(ctx, req)
}

// methodHandler decodes one request, calls the typed Runtime method,
// and encodes the result. Every business method has the same shape,
// so they share one signature and route through [methodTable] instead
// of a long switch (CLAUDE.md: 查表法替代条件链).
type methodHandler = func(*Dispatcher, context.Context, *transport.Request) HandleResult

// methodTable maps each JSON-RPC method name to its handler. Handlers
// live in domain-grouped files (handlers_runs.go / handlers_sessions.go
// / handlers_workspace.go / handlers_catalog.go); adding a method means
// writing one handler + one table row — nothing in dispatchRequest
// changes. Notifications (shutdown / canceled) are NOT here; they have
// no response and route through [Dispatcher.handleNotification].
var methodTable = map[string]methodHandler{
	// Lifecycle.
	MethodInitialize: (*Dispatcher).handleInitialize,
	MethodPing:       (*Dispatcher).handlePing,

	// Runs.
	MethodRunsStart:          (*Dispatcher).handleRunsStart,
	MethodRunsCancel:         (*Dispatcher).handleRunsCancel,
	MethodRunsApprovalSubmit: (*Dispatcher).handleRunsApprovalSubmit,

	// Sessions.
	MethodSessionsList:   (*Dispatcher).handleSessionsList,
	MethodSessionsGet:    (*Dispatcher).handleSessionsGet,
	MethodSessionsCreate: (*Dispatcher).handleSessionsCreate,
	MethodSessionsUpdate: (*Dispatcher).handleSessionsUpdate,
	MethodSessionsDelete: (*Dispatcher).handleSessionsDelete,
	MethodSessionsFork:   (*Dispatcher).handleSessionsFork,
	MethodSessionsExport: (*Dispatcher).handleSessionsExport,

	// Messages.
	MethodMessagesList: (*Dispatcher).handleMessagesList,
	MethodMessagesEdit: (*Dispatcher).handleMessagesEdit,

	// Workspace.
	MethodWorkspaceFilesChanged: (*Dispatcher).handleWorkspaceFilesChanged,
	MethodWorkspaceDiff:         (*Dispatcher).handleWorkspaceDiff,
	MethodWorkspaceFileHead:     (*Dispatcher).handleWorkspaceFileHead,
	MethodWorkspaceGrep:         (*Dispatcher).handleWorkspaceGrep,
	MethodWorkspaceProjects:     (*Dispatcher).handleWorkspaceProjects,
	MethodWorkspaceMCPList:      (*Dispatcher).handleWorkspaceMCPList,
	MethodWorkspaceMCPReconnect: (*Dispatcher).handleWorkspaceMCPReconnect,
	MethodWorkspaceSkills:       (*Dispatcher).handleWorkspaceSkills,

	// Providers / Models / Tools.
	MethodProvidersList:      (*Dispatcher).handleProvidersList,
	MethodProvidersTest:      (*Dispatcher).handleProvidersTest,
	MethodProvidersConfigure: (*Dispatcher).handleProvidersConfigure,
	MethodModelsList:         (*Dispatcher).handleModelsList,
	MethodToolsList:          (*Dispatcher).handleToolsList,
	MethodToolsInvoke:        (*Dispatcher).handleToolsInvoke,

	// Memory.
	MethodMemoryList:   (*Dispatcher).handleMemoryList,
	MethodMemoryGet:    (*Dispatcher).handleMemoryGet,
	MethodMemoryUpdate: (*Dispatcher).handleMemoryUpdate,

	// Attachments.
	MethodAttachmentsCreateUploadURL: (*Dispatcher).handleAttachmentsCreateUploadURL,
	MethodAttachmentsDelete:          (*Dispatcher).handleAttachmentsDelete,

	// Background.
	MethodBackgroundList: (*Dispatcher).handleBackgroundList,
	MethodBackgroundStop: (*Dispatcher).handleBackgroundStop,

	// Feedback.
	MethodFeedbackSubmit: (*Dispatcher).handleFeedbackSubmit,
}

// dispatchRequest routes the request to its handler via [methodTable].
// Unknown methods get -32601 method-not-found.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	handle, ok := methodTable[msg.Method]
	if !ok {
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
	return handle(d, ctx, msg)
}

// handleNotification dispatches the no-response methods. Errors are
// not surfaced over the wire (JSON-RPC notifications are fire-and-
// forget); the transport may log them.
//
// API.md v4 §2.4 / §3.5: notifications/canceled aborts an in-flight
// JSON-RPC Request (matched by requestId == Message.id). The
// dispatcher itself has no per-id ctx registry — the transport layer
// owns request lifecycle and must intercept this notification
// upstream of Handle. We accept the message here for protocol
// completeness; a future intercepting hook can plug in without
// changing the wire shape.
func (d *Dispatcher) handleNotification(ctx context.Context, msg *transport.Request) {
	switch msg.Method {
	case MethodShutdown:
		var in protocol.ShutdownRequest
		_ = unmarshal(msg.Params, &in)
		_ = d.api.Shutdown(ctx, in)

	case NotificationCancelled:
		// no-op at this layer (see method doc)
	}
}

// ─── helpers ────────────────────────────────────────────────────────

func responseResult(id transport.ID, result any) HandleResult {
	resp, err := transport.NewResponseResult(id, result)
	if err != nil {
		return responseError(id, transport.NewError(transport.CodeInternalError, problemDataFrom(err)))
	}
	return HandleResult{Response: resp}
}

func responseError(id transport.ID, rpcErr *transport.Error) HandleResult {
	return HandleResult{Response: transport.NewResponseError(id, rpcErr)}
}

// streamingResult is the same as [responseResult] but also attaches
// the run/task id + event channel so the transport's notification
// pump can fan events out under the resource id.
func streamingResult(id transport.ID, result any, runID string, events <-chan protocol.AgUiEvent, results <-chan protocol.RunResult) HandleResult {
	res := responseResult(id, result)
	res.RunID = runID
	res.EventStream = events
	res.ResultStream = results
	return res
}

func unmarshal(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// decodeIDParam pulls a single id-shaped string out of a method's
// params object. key parameterises which JSON field to look at so
// methods that name their id field differently (e.g. background.stop
// uses "taskId") share one decoder.
func decodeIDParam(raw json.RawMessage, key string) (string, error) {
	var obj map[string]string
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v := obj[key]; v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("missing %s", key)
}
