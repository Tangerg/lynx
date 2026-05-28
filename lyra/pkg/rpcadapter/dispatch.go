package rpcadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// Dispatcher routes inbound JSON-RPC messages to typed CoreAPI
// methods. One instance per connection — it carries the per-conn
// handshake state (initialized + negotiated capabilities) which the
// HTTP transport keys by request affinity (cookie / sticky token) or
// the InProcess transport sees as a single long-lived value.
type Dispatcher struct {
	api coreapi.CoreAPI

	// initialized flips once runtime.initialize succeeds. Until then
	// every business method returns -32011 protocol_violation. atomic
	// so concurrent dispatch goroutines see a consistent view.
	initialized atomic.Bool
}

// New builds a Dispatcher bound to the given CoreAPI. Concurrency:
// the returned Dispatcher is safe for parallel Handle calls; per-conn
// state lives on it, so use one Dispatcher per logical session.
func New(api coreapi.CoreAPI) *Dispatcher {
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
	EventStream <-chan coreapi.AgUiEvent
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

// dispatchRequest fans the request out to the right CoreAPI method.
// Each method shape is small and self-explanatory — decode params,
// call the typed method, encode result.
func (d *Dispatcher) dispatchRequest(ctx context.Context, msg *transport.Request) HandleResult {
	switch msg.Method {

	// ─── Lifecycle ──────────────────────────────────────────────
	case MethodInitialize:
		var in coreapi.InitializeIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		out, err := d.api.Initialize(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		d.initialized.Store(true)
		return responseResult(msg.ID, out)

	case MethodPing:
		if err := d.api.Ping(ctx); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	// ─── Runs ───────────────────────────────────────────────────
	case MethodRunsStart:
		var in coreapi.StartRunIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		out, events, err := d.api.StartRun(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return streamingResult(msg.ID, out, out.RunID, events)

	case MethodRunsCancel:
		// API.md v4 §3.5: runs.cancel is a Request (not a notification).
		// It stops an in-flight run identified by runId. Decoupled from
		// notifications/cancelled (which aborts an in-flight JSON-RPC
		// Request — different semantic).
		var in coreapi.CancelRunIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if in.RunID == "" {
			return responseError(msg.ID, invalidParams("runId is required"))
		}
		if err := d.api.CancelRun(ctx, in.RunID); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	case MethodRunsApprovalSubmit:
		var in coreapi.ApprovalIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.SubmitApproval(ctx, in); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	// ─── Sessions ───────────────────────────────────────────────
	case MethodSessionsList:
		var q coreapi.PageQuery
		_ = unmarshal(msg.Params, &q) // empty params is valid
		page, err := d.api.ListSessions(ctx, q)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, page)

	case MethodSessionsGet:
		id, err := decodeIDParam(msg.Params, "id")
		if err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		sess, err := d.api.GetSession(ctx, id)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, sess)

	case MethodSessionsCreate:
		var in coreapi.CreateSessionIn
		_ = unmarshal(msg.Params, &in)
		sess, err := d.api.CreateSession(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, sess)

	case MethodSessionsUpdate:
		// UpdateSessionIn is flat — `id` lives alongside the patch
		// fields. One unmarshal pass covers everything (no inline-tag
		// hack).
		var in coreapi.UpdateSessionIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if in.ID == "" {
			return responseError(msg.ID, invalidParams("id is required"))
		}
		sess, err := d.api.UpdateSession(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, sess)

	case MethodSessionsDelete:
		id, err := decodeIDParam(msg.Params, "id")
		if err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.DeleteSession(ctx, id); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	case MethodSessionsFork:
		var in coreapi.ForkSessionIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		sess, err := d.api.ForkSession(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, sess)

	case MethodSessionsExport:
		var in coreapi.ExportSessionIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		out, err := d.api.ExportSession(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, out)

	// ─── Messages ───────────────────────────────────────────────
	case MethodMessagesList:
		// Flat shape: sessionId + limit + cursor at the same level.
		var in coreapi.ListMessagesIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if in.SessionID == "" {
			return responseError(msg.ID, invalidParams("sessionId is required"))
		}
		page, err := d.api.ListMessages(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, page)

	case MethodMessagesEdit:
		var in coreapi.EditMessageIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		out, err := d.api.EditMessage(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, out)

	// ─── Workspace ──────────────────────────────────────────────
	// Non-paginated list methods return bare arrays (no {items}
	// wrapper) per API.md v3 §5.2.
	case MethodWorkspaceFilesChanged:
		files, err := d.api.WorkspaceFilesChanged(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, files)

	case MethodWorkspaceDiff:
		var in struct {
			Path string `json:"path"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		rows, err := d.api.WorkspaceDiff(ctx, in.Path)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, rows)

	case MethodWorkspaceFileHead:
		var in struct {
			Path string `json:"path"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		lines, err := d.api.WorkspaceFileHead(ctx, in.Path)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, lines)

	case MethodWorkspaceGrep:
		var in struct {
			Query string `json:"query"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		res, err := d.api.WorkspaceGrep(ctx, in.Query)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, res)

	case MethodWorkspaceProjects:
		projects, err := d.api.WorkspaceProjects(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, projects)

	case MethodWorkspaceMCPList:
		servers, err := d.api.WorkspaceMCPList(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, servers)

	case MethodWorkspaceMCPReconnect:
		var in struct {
			Name string `json:"name"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.WorkspaceMCPReconnect(ctx, in.Name); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	case MethodWorkspaceSkills:
		skills, err := d.api.WorkspaceSkills(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, skills)

	// ─── Providers / Models / Tools ─────────────────────────────
	case MethodProvidersList:
		providers, err := d.api.ListProviders(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, providers)

	case MethodProvidersTest:
		var in struct {
			ID string `json:"id"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		res, err := d.api.TestProvider(ctx, in.ID)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, res)

	case MethodModelsList:
		var in struct {
			Provider string `json:"provider"`
		}
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		models, err := d.api.ListModels(ctx, in.Provider)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, models)

	case MethodToolsList:
		tools, err := d.api.ListTools(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, tools)

	// ─── Attachments ────────────────────────────────────────────
	case MethodAttachmentsCreateUploadURL:
		var in coreapi.CreateUploadURLIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		out, err := d.api.CreateUploadURL(ctx, in)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, out)

	case MethodAttachmentsDelete:
		id, err := decodeIDParam(msg.Params, "id")
		if err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.DeleteAttachment(ctx, id); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	// ─── Background ─────────────────────────────────────────────
	case MethodBackgroundList:
		tasks, err := d.api.ListBackground(ctx)
		if err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, tasks)

	case MethodBackgroundStop:
		// API.md v3 §4.1: param key is `taskId`, not generic `id`.
		taskID, err := decodeIDParam(msg.Params, "taskId")
		if err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.StopBackground(ctx, taskID); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	// ─── Feedback ───────────────────────────────────────────────
	case MethodFeedbackSubmit:
		var in coreapi.FeedbackIn
		if err := unmarshal(msg.Params, &in); err != nil {
			return responseError(msg.ID, invalidParams(err.Error()))
		}
		if err := d.api.SubmitFeedback(ctx, in); err != nil {
			return responseError(msg.ID, errorToRPC(err))
		}
		return responseResult(msg.ID, struct{}{})

	default:
		return responseError(msg.ID, methodNotFound(msg.Method))
	}
}

// handleNotification dispatches the no-response methods. Errors are
// not surfaced over the wire (JSON-RPC notifications are fire-and-
// forget); the transport may log them.
//
// API.md v4 §2.4 / §3.5: notifications/cancelled aborts an in-flight
// JSON-RPC Request (matched by requestId == Message.id). The
// dispatcher itself has no per-id ctx registry — the transport layer
// owns request lifecycle and must intercept this notification
// upstream of Handle. We accept the message here for protocol
// completeness; a future intercepting hook can plug in without
// changing the wire shape.
func (d *Dispatcher) handleNotification(ctx context.Context, msg *transport.Request) {
	switch msg.Method {
	case MethodShutdown:
		var in coreapi.ShutdownIn
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
func streamingResult(id transport.ID, result any, runID string, events <-chan coreapi.AgUiEvent) HandleResult {
	res := responseResult(id, result)
	res.RunID = runID
	res.EventStream = events
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

