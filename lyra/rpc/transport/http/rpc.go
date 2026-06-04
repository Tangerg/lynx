package http

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// maxRPCBodyBytes caps the JSON-RPC request body to avoid trivial
// DoS via a giant payload. 4 MB matches typical reasonable runs.start
// histories without trimming.
const maxRPCBodyBytes = 4 << 20

// handleRPCWithMethod is the single JSON-RPC entry point —
// `POST /v2/rpc/{method}` per TRANSPORT §3. The URL method is
// cross-checked against the body's method (mismatch ⇒ invalid_request / 400).
func (s *Server) handleRPCWithMethod(w http.ResponseWriter, r *http.Request) {
	// chi only routes /v2/rpc/{method} with a non-empty single segment;
	// bare /v2/rpc has no matching route and 404s before reaching here.
	s.serveRPC(w, r, chi.URLParam(r, "method"))
}

// serveRPC reads, dispatches, and serializes one JSON-RPC message.
// Wire encode/decode goes through transport.DecodeMessage /
// EncodeMessage — those wrap the MCP SDK's jsonrpc package, which
// owns the conformant JSON-RPC 2.0 implementation.
func (s *Server) serveRPC(w http.ResponseWriter, r *http.Request, urlMethod string) {
	// Transport-layer preconditions (TRANSPORT §6.3): unsupported media
	// type ⇒ 415, oversized body ⇒ 413 — both rejected before we spend
	// effort decoding. Content-Type is only enforced when present (a
	// minimal client may omit it); when set it must be application/json.
	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !isJSONMediaType(ct) {
		writeFlatError(w, r, http.StatusUnsupportedMediaType, "content-type must be application/json", false)
		return
	}
	if r.ContentLength > maxRPCBodyBytes {
		writeFlatError(w, r, http.StatusRequestEntityTooLarge, "request body exceeds limit", false)
		return
	}

	// Read one byte past the cap so a chunked / uncounted body that
	// overflows surfaces as 413 rather than silently truncating.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRPCBodyBytes+1))
	if err != nil {
		writeFlatError(w, r, http.StatusBadRequest, "read body: "+err.Error(), false)
		return
	}
	if len(body) > maxRPCBodyBytes {
		writeFlatError(w, r, http.StatusRequestEntityTooLarge, "request body exceeds limit", false)
		return
	}

	msg, err := transport.DecodeMessage(body)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, transport.ID{},
			transport.NewError(transport.CodeParseError,
				marshalProblem("invalid JSON-RPC envelope: "+err.Error())))
		return
	}

	// Carry the streaming reconnect cursor (Last-Event-Id) out-of-band on
	// the ctx so runs.subscribe replays a run's durable backlog from that
	// point rather than re-sending it whole (TRANSPORT §9.2). Harmless for
	// non-streaming methods (they don't read it).
	ctx := transport.WithLastEventID(r.Context(), strings.TrimSpace(r.Header.Get("Last-Event-Id")))
	res := s.dispatcher.Handle(ctx, msg, urlMethod)

	// Surface the body's method (if any) for the X-Method header.
	// Only Request envelopes carry Method; Responses don't.
	bodyMethod := ""
	if req, ok := msg.(*transport.Request); ok {
		bodyMethod = req.Method
	}

	methodLabel := chooseMethodLabel(urlMethod, bodyMethod)

	// Client notifications are dispatched synchronously and acknowledged
	// with 204 No Content — no body (TRANSPORT §6.3 explicitly picks 204
	// over 202, since processing is already complete, not pending).
	if res.Response == nil {
		w.Header().Set("X-Server", s.serverID)
		if methodLabel != "" {
			w.Header().Set("X-Method", methodLabel)
		}
		echoTraceID(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Streaming method (stream opened) → the POST response body IS the
	// event stream (streamable HTTP, TRANSPORT §6.4). A pre-stream failure
	// (session_not_found / invalid_params …) leaves EventStream nil and
	// falls through to the single-shot application/json reply below — §6.2.
	if res.EventStream != nil {
		s.serveStream(w, r, res.Response, res.EventStream, methodLabel)
		return
	}

	// Non-streaming → one application/json JSON-RPC response. HTTP status
	// per TRANSPORT §6.3: 200 by default, 404 on method-not-found, 400 on
	// invalid request / parse error / URL-body method mismatch.
	status := statusForRPC(res.Response, urlMethod)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Server", s.serverID)
	if methodLabel != "" {
		w.Header().Set("X-Method", methodLabel)
	}
	echoTraceID(w, r)
	w.WriteHeader(status)
	data, err := transport.EncodeMessage(res.Response)
	if err != nil {
		recordError(r.Context(), "rpc.encode-response", err,
			attribute.String("lynx.lyra.method", methodLabel),
		)
		return
	}
	if _, err := w.Write(data); err != nil {
		recordError(r.Context(), "rpc.write-response", err,
			attribute.String("lynx.lyra.method", methodLabel),
		)
	}
}

// statusForRPC translates a dispatcher response into an HTTP status
// (TRANSPORT §6.3).
func statusForRPC(resp *transport.Response, urlMethod string) int {
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
	case transport.CodeParseError:
		return http.StatusBadRequest
	case transport.CodeInvalidRequest:
		// Any malformed envelope — including a URL/body method mismatch
		// (a self-contradictory request, not a resource conflict) — is
		// 400, not 409 (TRANSPORT §6.3).
		return http.StatusBadRequest
	case transport.CodeMethodNotFound:
		// URL-form is the only path, so unknown methods always come in
		// with urlMethod set ⇒ 404 is the right answer.
		if urlMethod != "" {
			return http.StatusNotFound
		}
	}
	return http.StatusOK
}

// chooseMethodLabel returns the method name to surface in the
// X-Method response header. URL form wins when present; body
// method is the fallback (defensive — should always be set today).
func chooseMethodLabel(urlMethod, bodyMethod string) string {
	if urlMethod != "" {
		return urlMethod
	}
	return bodyMethod
}

// writeFlatError serves 4xx/5xx responses that originate BELOW the
// JSON-RPC layer (bad path, unread body, failed auth, dead stream) — a
// flat JSON envelope `{"error", "traceId"?}` (TRANSPORT §6.3), NOT the
// JSON-RPC envelope, since there may be no valid request id. X-Trace-Id
// is echoed into the body's `traceId` for ops correlation. noCache adds
// Cache-Control: no-store (auth failures must not be cached).
func writeFlatError(w http.ResponseWriter, r *http.Request, status int, msg string, noCache bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if noCache {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	body := struct {
		Error   string `json:"error"`
		TraceID string `json:"traceId,omitempty"`
	}{Error: msg}
	if r != nil {
		body.TraceID = strings.TrimSpace(r.Header.Get("X-Trace-Id"))
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

// echoTraceID copies the client-supplied X-Trace-Id into the
// response's X-Trace-Id header (TRANSPORT §16).
func echoTraceID(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimSpace(r.Header.Get("X-Trace-Id"))
	if traceID == "" {
		return
	}
	w.Header().Set("X-Trace-Id", traceID)
}

// isJSONMediaType reports whether a Content-Type header denotes JSON.
// It tolerates parameters (e.g. "application/json; charset=utf-8") by
// parsing off the media type before comparing (TRANSPORT §6.3 / §6.2).
func isJSONMediaType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "application/json"
}

// marshalProblem wraps detail in a protocol.ProblemData JSON blob
// (API.md §8). Used for transport-level parse errors where there's no
// runtime sentinel to classify.
func marshalProblem(detail string) json.RawMessage {
	body, _ := json.Marshal(protocol.ProblemData{Type: "parse_error", Detail: detail})
	return body
}
