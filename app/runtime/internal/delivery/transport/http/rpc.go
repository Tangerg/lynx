package http

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// maxRPCBodyBytes caps the JSON-RPC request body to avoid trivial
// DoS via a giant payload. 4 MB matches typical reasonable runs.start
// histories without trimming.
const maxRPCBodyBytes = 4 << 20

// serveRPC reads, dispatches, and serializes one JSON-RPC message.
// Wire encode/decode goes through transport.DecodeMessage /
// EncodeMessage — those wrap the MCP SDK's jsonrpc package, which
// owns the conformant JSON-RPC 2.0 implementation.
func (s *Server) serveRPC(w http.ResponseWriter, r *http.Request) {
	// Transport-layer preconditions (TRANSPORT §6.3): unsupported media
	// type ⇒ 415, oversized body ⇒ 413 — both rejected before we spend
	// effort decoding. Content-Type is only enforced when present (a
	// minimal client may omit it); when set it must be application/json.
	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !isJSONMediaType(ct) {
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "content-type must be application/json", false)
		return
	}
	if r.ContentLength > maxRPCBodyBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds limit", false)
		return
	}

	// Read one byte past the cap so a chunked / uncounted body that
	// overflows surfaces as 413 rather than silently truncating.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRPCBodyBytes+1))
	if err != nil {
		recordError(r.Context(), "rpc.read-request", err)
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body could not be read", false)
		return
	}
	if len(body) > maxRPCBodyBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds limit", false)
		return
	}

	msg, err := transport.DecodeMessage(body)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "invalid JSON-RPC message: "+err.Error(), false)
		return
	}

	// Carry the streaming reconnect cursor (Last-Event-Id) out-of-band on
	// the ctx so runs.subscribe replays a run's durable backlog from that
	// point rather than re-sending it whole (TRANSPORT §9.2). Harmless for
	// non-streaming methods (they don't read it).
	ctx := transport.WithLastEventID(r.Context(), strings.TrimSpace(r.Header.Get("Last-Event-Id")))
	ctx = transport.WithIdempotencyKey(ctx, strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	res := s.dispatcher.Handle(ctx, msg)

	// Surface the body's method (if any) for the X-Method header.
	// Only Request envelopes carry Method; Responses don't.
	bodyMethod := ""
	if req, ok := msg.(*transport.Request); ok {
		bodyMethod = req.Method
	}

	methodLabel := bodyMethod

	// Client notifications are dispatched synchronously and acknowledged
	// with 204 No Content — no body (TRANSPORT §6.3 explicitly picks 204
	// over 202, since processing is already complete, not pending).
	if res.Response == nil {
		if methodLabel != "" {
			w.Header().Set("X-Method", methodLabel)
		}
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

	// A decoded JSON-RPC call always returns 200. Its result or error belongs to
	// the envelope; HTTP status is reserved for failures below this boundary.
	data, err := transport.EncodeMessage(res.Response)
	if err != nil {
		recordError(r.Context(), "rpc.encode-response", err,
			attribute.String("rpc.method", methodLabel),
		)
		writeProblem(w, http.StatusInternalServerError, "response_encoding_failed", "the transport could not encode the RPC response", false)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if methodLabel != "" {
		w.Header().Set("X-Method", methodLabel)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		recordError(r.Context(), "rpc.write-response", err,
			attribute.String("rpc.method", methodLabel),
		)
	}
}

type transportProblem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	RequestID string `json:"requestId,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, typ, detail string, noCache bool) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	if noCache {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(transportProblem{
		Type:      "urn:lyra:transport:" + typ,
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		RequestID: w.Header().Get("Request-Id"),
	})
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
