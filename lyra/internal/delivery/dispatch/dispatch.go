package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/delivery/transport"
)

// Dispatcher routes inbound JSON-RPC messages to typed Runtime
// methods. One instance per connection — it carries the per-conn
// handshake state (initialized) which the HTTP transport keys by
// request affinity and the InProcess transport sees as a single
// long-lived value.
type Dispatcher struct {
	api protocol.Runtime

	// initialized flips once runtime.initialize succeeds. Until then
	// every business method returns capability_not_negotiated. atomic
	// so concurrent dispatch goroutines see a consistent view.
	initialized atomic.Bool
}

// New builds a Dispatcher bound to the given Runtime. The returned
// Dispatcher is safe for parallel Handle calls; per-conn state lives on
// it, so use one Dispatcher per logical connection.
func New(api protocol.Runtime) *Dispatcher {
	return &Dispatcher{api: api}
}

// HandleResult holds what the dispatcher returns after processing one
// inbound message.
type HandleResult struct {
	// Response is the synchronous JSON-RPC reply. nil when the input
	// was a notification (no id, no response on the wire).
	Response *transport.Response

	// EventStream is the channel of stream frames for a streaming method,
	// closed when the stream ends. Frames are domain-agnostic (run events,
	// workspace events): the dispatch encodes each domain event into a
	// StreamFrame (method + params + optional SSE id) so the transport stays
	// dumb — it just writes frames. Under streamable HTTP the transport drains
	// it straight into the call's own text/event-stream response.
	EventStream <-chan StreamFrame
}

// StreamFrame is one ready-to-write downstream notification on a streaming
// method's event channel. The dispatch produces these from domain events so
// every transport writes them identically. SSEID drives Last-Event-Id replay;
// "" marks an ephemeral frame (no replay) — e.g. all workspace events.
type StreamFrame struct {
	Notif transport.Message
	SSEID string
}

// Handle is the entry point — every inbound transport.Message goes
// through here. ExpectedMethod, when non-empty, is the method the
// transport extracted from the URL path (POST /v2/rpc/{method}); the
// dispatcher cross-checks it against the body method.
func (d *Dispatcher) Handle(ctx context.Context, msg transport.Message, expectedMethod string) HandleResult {
	req, ok := msg.(*transport.Request)
	if !ok || req == nil {
		return responseError(transport.ID{}, badEnvelope("expected a JSON-RPC request"))
	}

	if expectedMethod != "" && expectedMethod != req.Method {
		return responseError(req.ID, badEnvelope(fmt.Sprintf(
			"url method %q does not match body method %q", expectedMethod, req.Method,
		)))
	}

	// API.md §2.2: all ids are strings. Reject non-string ids at the
	// boundary. (Absent ids — Notifications — are fine.)
	if req.ID.IsValid() {
		if _, ok := req.ID.Raw().(string); !ok {
			return responseError(req.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", req.ID.Raw())))
		}
	}

	// Notifications: no response. We still dispatch so cancel-style
	// notifications take effect.
	if !req.IsCall() {
		d.handleNotification(ctx, req)
		return HandleResult{}
	}

	// Gate business methods behind initialize.
	if !d.initialized.Load() && req.Method != MethodInitialize && req.Method != MethodPing {
		return responseError(req.ID, notInitialized(
			"runtime.initialize must succeed before any business method"))
	}

	return d.dispatchRequest(ctx, req)
}

// methodHandler decodes one request, calls the typed Runtime method,
// and encodes the result. Every business method shares this signature
// and routes through [methodTable] (CLAUDE.md: 查表法替代条件链).
type methodHandler = func(*Dispatcher, context.Context, *transport.Request) HandleResult

// methodTable maps each JSON-RPC method name to its handler. Handlers
// live in domain-grouped files; adding a method = one handler + one
// row. Notifications route through [Dispatcher.handleNotification].
var methodTable = map[string]methodHandler{
	// Lifecycle.
	MethodInitialize: (*Dispatcher).handleInitialize,
	MethodPing:       (*Dispatcher).handlePing,

	// Sessions.
	MethodSessionsList:     (*Dispatcher).handleSessionsList,
	MethodSessionsGet:      (*Dispatcher).handleSessionsGet,
	MethodSessionsCreate:   (*Dispatcher).handleSessionsCreate,
	MethodSessionsUpdate:   (*Dispatcher).handleSessionsUpdate,
	MethodSessionsDelete:   (*Dispatcher).handleSessionsDelete,
	MethodSessionsFork:     (*Dispatcher).handleSessionsFork,
	MethodSessionsRollback: (*Dispatcher).handleSessionsRollback,
	MethodSessionsExport:   (*Dispatcher).handleSessionsExport,
	MethodSessionsImport:   (*Dispatcher).handleSessionsImport,

	// Runs.
	MethodRunsStart:              (*Dispatcher).handleRunsStart,
	MethodRunsResume:             (*Dispatcher).handleRunsResume,
	MethodRunsSubscribe:          (*Dispatcher).handleRunsSubscribe,
	MethodRunsCancel:             (*Dispatcher).handleRunsCancel,
	MethodRunsList:               (*Dispatcher).handleRunsList,
	MethodRunsListOpenInterrupts: (*Dispatcher).handleRunsListOpenInterrupts,

	// Items.
	MethodItemsList: (*Dispatcher).handleItemsList,

	// Workspace.
	MethodWorkspaceListFileChanges: (*Dispatcher).handleWorkspaceListFileChanges,
	MethodWorkspaceGetDiff:         (*Dispatcher).handleWorkspaceGetDiff,
	MethodWorkspaceGetFileHead:     (*Dispatcher).handleWorkspaceGetFileHead,
	MethodWorkspaceGrep:            (*Dispatcher).handleWorkspaceGrep,
	MethodWorkspaceListProjects:    (*Dispatcher).handleWorkspaceListProjects,
	MethodWorkspaceListSkills:      (*Dispatcher).handleWorkspaceListSkills,
	MethodWorkspaceListAgentDocs:   (*Dispatcher).handleWorkspaceListAgentDocs,
	MethodWorkspaceMCPListServers:  (*Dispatcher).handleWorkspaceMCPListServers,
	MethodWorkspaceMCPListTools:    (*Dispatcher).handleWorkspaceMCPListTools,
	MethodWorkspaceMCPReconnect:    (*Dispatcher).handleWorkspaceMCPReconnect,
	MethodWorkspaceSubscribe:       (*Dispatcher).handleWorkspaceSubscribe,

	// Providers / Models / Tools.
	MethodProvidersList:      (*Dispatcher).handleProvidersList,
	MethodProvidersConfigure: (*Dispatcher).handleProvidersConfigure,
	MethodProvidersTest:      (*Dispatcher).handleProvidersTest,
	MethodModelsList:         (*Dispatcher).handleModelsList,
	MethodToolsList:          (*Dispatcher).handleToolsList,
	MethodToolsInvoke:        (*Dispatcher).handleToolsInvoke,

	// Memory.
	MethodMemoryList:   (*Dispatcher).handleMemoryList,
	MethodMemoryGet:    (*Dispatcher).handleMemoryGet,
	MethodMemoryUpdate: (*Dispatcher).handleMemoryUpdate,

	// Attachments.
	MethodAttachmentsCreateUpload: (*Dispatcher).handleAttachmentsCreateUpload,
	MethodAttachmentsGet:          (*Dispatcher).handleAttachmentsGet,
	MethodAttachmentsDelete:       (*Dispatcher).handleAttachmentsDelete,

	// Feedback.
	MethodFeedbackCreate: (*Dispatcher).handleFeedbackCreate,
}

// dispatchRequest routes the request to its handler via [methodTable].
// Unknown methods get method_not_found.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	handle, ok := methodTable[msg.Method]
	if !ok {
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
	return handle(d, ctx, msg)
}

// handleNotification dispatches the no-response methods. Errors are not
// surfaced over the wire (JSON-RPC notifications are fire-and-forget).
//
// notifications.canceled aborts an in-flight JSON-RPC request (matched
// by the carried envelope id); the transport layer owns request
// lifecycle and intercepts it upstream of Handle. We accept it here for
// protocol completeness.
func (d *Dispatcher) handleNotification(ctx context.Context, msg *transport.Request) {
	switch msg.Method {
	case MethodShutdown:
		var in protocol.ShutdownRequest
		_ = unmarshal(msg.Params, &in)
		_ = d.api.Shutdown(ctx, in)
	case NotificationCanceled:
		// no-op at this layer (see method doc)
	}
}

// ─── helpers ────────────────────────────────────────────────────────

func responseResult(id transport.ID, result any) HandleResult {
	resp, err := transport.NewResponseResult(id, result)
	if err != nil {
		return responseError(id, problemError(protocol.CodeInternalError, "internal_error", err.Error()))
	}
	return HandleResult{Response: resp}
}

func responseError(id transport.ID, rpcErr *transport.Error) HandleResult {
	return HandleResult{Response: transport.NewResponseError(id, rpcErr)}
}

// streamingResult attaches the frame channel onto the synchronous reply;
// the transport streams it as the call's own response (streamable HTTP).
func streamingResult(id transport.ID, result any, events <-chan StreamFrame) HandleResult {
	res := responseResult(id, result)
	res.EventStream = events
	return res
}

// adaptStream fans a domain event channel into a StreamFrame channel via conv,
// which encodes each event (returns ok=false to skip an unencodable one). The
// goroutine exits on ctx cancellation OR when in closes, and never blocks past
// ctx (leak-safe): the streaming request's ctx ends on client disconnect /
// completion, which also stops the source.
func adaptStream[T any](ctx context.Context, in <-chan T, conv func(T) (StreamFrame, bool)) <-chan StreamFrame {
	out := make(chan StreamFrame)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
				frame, ok := conv(ev)
				if !ok {
					continue
				}
				select {
				case out <- frame:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// runEventToFrame encodes a RunEvent into a notifications.run.event frame.
// Only durable events carry an SSE id: (TRANSPORT §9.3 / API §5.2) — replay
// must resume from a replayable event, never an ephemeral delta.
func runEventToFrame(ev protocol.RunEvent) (StreamFrame, bool) {
	notif, err := EncodeRunEvent(ev)
	if err != nil {
		return StreamFrame{}, false
	}
	sseID := ""
	if ev.Event.IsDurable() {
		sseID = ev.EventID
	}
	return StreamFrame{Notif: notif, SSEID: sseID}, true
}

func unmarshal(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// decodeStringParam pulls a single named string field out of a method's
// params object. key parameterises which JSON field to read so methods
// that name their id field differently share one decoder.
func decodeStringParam(raw json.RawMessage, key string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("dispatch.decodeStringParam: missing %s", key)
}
