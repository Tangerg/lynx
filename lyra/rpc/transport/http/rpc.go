package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/dispatch"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// maxRPCBodyBytes caps the JSON-RPC request body to avoid trivial
// DoS via a giant payload. 4 MB matches typical reasonable runs.start
// histories without trimming.
const maxRPCBodyBytes = 4 << 20

// handleRPCWithMethod is the single JSON-RPC entry point —
// `POST /v1/rpc/{method}` per API.md v4 §10.1. The URL method is
// cross-checked against the body's method (mismatch ⇒ -32011 / 409).
//
// Go's `{method...}` wildcard matches the bare `/v1/rpc` path too
// (zero-or-more trailing segments), so we explicitly 404 on the
// empty-method case to honour the v4 "no fallback" rule.
func (s *Server) handleRPCWithMethod(w http.ResponseWriter, r *http.Request) {
	urlMethod := r.PathValue("method")
	if urlMethod == "" {
		writeTransportError(w, r, http.StatusNotFound,
			"POST /v1/rpc requires a method suffix (use /v1/rpc/{method})")
		return
	}
	s.serveRPC(w, r, urlMethod)
}

// serveRPC reads, dispatches, and serialises one JSON-RPC message.
// Wire encode/decode goes through transport.DecodeMessage /
// EncodeMessage — those wrap the MCP SDK's jsonrpc package, which
// owns the conformant JSON-RPC 2.0 implementation.
func (s *Server) serveRPC(w http.ResponseWriter, r *http.Request, urlMethod string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRPCBodyBytes))
	if err != nil {
		writeTransportError(w, r, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	msg, err := transport.DecodeMessage(body)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, transport.ID{},
			transport.NewError(transport.CodeParseError,
				marshalProblem("invalid JSON-RPC envelope: "+err.Error())))
		return
	}

	res := s.dispatcher.Handle(r.Context(), msg, urlMethod)

	// Surface the body's method (if any) for the X-Lyra-Method header.
	// Only Request envelopes carry Method; Responses don't.
	bodyMethod := ""
	if req, ok := msg.(*transport.Request); ok {
		bodyMethod = req.Method
	}

	// Notifications get 204 No Content per API.md §7.3.
	if res.Response == nil {
		w.Header().Set("X-Lyra-Server", s.serverID)
		if urlMethod != "" {
			w.Header().Set("X-Lyra-Method", urlMethod)
		} else if bodyMethod != "" {
			w.Header().Set("X-Lyra-Method", bodyMethod)
		}
		echoTraceID(w, r)
		if res.EventStream != nil {
			s.attachStream(r.Context(), res.RunID, res.EventStream)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Streaming response: kick off the event pump in the background.
	if res.EventStream != nil {
		s.attachStream(context.Background(), res.RunID, res.EventStream)
	}

	// Compute HTTP status (API.md §7.3): 200 by default, 404 on
	// method-not-found, 409 on URL/body method mismatch, 400 on
	// invalid request / parse error.
	status := statusForRPC(res.Response, urlMethod, bodyMethod)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Lyra-Server", s.serverID)
	methodLabel := chooseMethodLabel(urlMethod, bodyMethod)
	if methodLabel != "" {
		w.Header().Set("X-Lyra-Method", methodLabel)
	}
	echoTraceID(w, r)
	w.WriteHeader(status)
	if data, err := transport.EncodeMessage(res.Response); err == nil {
		_, _ = w.Write(data)
	}
}

// attachStream registers the AG-UI event stream with the per-stream
// replay buffer (keyed by runId, API.md v4 §3.1) and the client
// broadcast registry. Events arrive through dispatch.EncodeRunEvent.
func (s *Server) attachStream(ctx context.Context, runID string, events <-chan protocol.AgUiEvent) {
	if runID == "" || events == nil {
		return
	}
	buf := s.streams.open(runID)
	go func() {
		var seq uint64
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					closed, _ := dispatch.EncodeRunClosed(runID, "completed")
					if closed != nil {
						s.clients.broadcast(closed)
					}
					// Keep the buffer alive for the replay window;
					// streamBuffer.append GC's by age.
					return
				}
				seq++
				eventID := strconv.FormatUint(seq, 10)
				notif, err := dispatch.EncodeRunEvent(runID, eventID, ev)
				if err != nil {
					continue
				}
				buf.append(streamRecord{eventID: eventID, msg: notif, at: nowFn()})
				s.clients.broadcast(notif)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// statusForRPC translates a dispatcher response into an HTTP status
// (API.md §7.3).
func statusForRPC(resp *transport.Response, urlMethod, bodyMethod string) int {
	if resp == nil || resp.Error == nil {
		return http.StatusOK
	}
	// resp.Error is `error` typed — but we always populate it with a
	// *transport.Error. Cast to inspect the code.
	rpcErr, ok := resp.Error.(*transport.Error)
	if !ok {
		return http.StatusInternalServerError
	}
	switch rpcErr.Code {
	case transport.CodeParseError, transport.CodeInvalidRequest:
		return http.StatusBadRequest
	case transport.CodeMethodNotFound:
		// URL-form is the only path, so unknown methods always come in
		// with urlMethod set ⇒ 404 is the right answer.
		if urlMethod != "" {
			return http.StatusNotFound
		}
		return http.StatusOK
	case transport.CodeProtocolViolation:
		if urlMethod != "" && bodyMethod != "" && urlMethod != bodyMethod {
			return http.StatusConflict
		}
		return http.StatusOK
	}
	return http.StatusOK
}

// chooseMethodLabel returns the method name to surface in the
// X-Lyra-Method response header. URL form wins when present; body
// method is the fallback (defensive — should always be set today).
func chooseMethodLabel(urlMethod, bodyMethod string) string {
	if urlMethod != "" {
		return urlMethod
	}
	return bodyMethod
}

// writeTransportError serves 4xx/5xx responses that originated below
// the JSON-RPC layer (request body could not be read, etc.). Per
// API.md §7.3, these use a flat JSON envelope — NOT the JSON-RPC
// envelope, since we may not have a valid request id.
//
// X-Lyra-Trace-Id is echoed into the body as `traceId` so the FE's
// RpcTransportError.traceId field gets populated for ops correlation.
func writeTransportError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body := struct {
		Error   string `json:"error"`
		TraceID string `json:"traceId,omitempty"`
	}{Error: msg}
	if r != nil {
		body.TraceID = strings.TrimSpace(r.Header.Get("X-Lyra-Trace-Id"))
	}
	_ = json.NewEncoder(w).Encode(body)
}

// writeRPCError serves a JSON-RPC error envelope with an explicit
// HTTP status. Used for parse errors where we successfully read the
// body but couldn't decode it as a message.
func writeRPCError(w http.ResponseWriter, status int, id transport.ID, rpcErr *transport.Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	resp := transport.NewResponseError(id, rpcErr)
	if data, err := transport.EncodeMessage(resp); err == nil {
		_, _ = w.Write(data)
	}
}

// echoTraceID copies the client-supplied X-Lyra-Trace-Id into the
// response's X-Lyra-Request-Id header (API.md §10.5).
func echoTraceID(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimSpace(r.Header.Get("X-Lyra-Trace-Id"))
	if traceID == "" {
		return
	}
	w.Header().Set("X-Lyra-Request-Id", traceID)
}

// marshalProblem wraps detail in a transport.ProblemData JSON blob.
func marshalProblem(detail string) json.RawMessage {
	body, _ := json.Marshal(transport.ProblemData{Detail: detail})
	return body
}
