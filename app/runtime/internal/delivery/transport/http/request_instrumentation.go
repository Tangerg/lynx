package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// instrumentRequests wraps the mux with the entry-point tracing layer — the
// root of full-link tracing:
//
//   - extract any W3C traceparent the client sent, then open one server span per
//     request. Downstream work inherits it through r.Context(), so a single
//     trace covers the whole request. The span carries HTTP attributes,
//     duration, and body size, and is marked Error on 5xx.
//   - panic recovery so the runtime survives a misbehaving handler; the panic
//     is recorded onto the request span, and an uncommitted response becomes a
//     flat 500 envelope without corrupting an already-started stream.
//
// All observability flows through OTel; the process composition root installs
// the global TracerProvider and propagator.
func (s *Server) instrumentRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Request-Id", newRequestID())
		w.Header().Set("X-Server", s.serverID)

		// Continue the client's trace when it sent one, else start fresh —
		// this is where the request's trace_id comes into being.
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
			),
		)
		r = r.WithContext(ctx)

		response := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			span.SetAttributes(
				attribute.Int("http.response.status_code", response.status),
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				attribute.Int("http.response.body.size", response.bytes),
			)
			if response.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(response.status))
			}
			span.End()
		}()

		defer func() {
			if recovered := recover(); recovered != nil {
				err := handlerPanicError(recovered)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				if !response.wroteHeader {
					writeProblem(response, http.StatusInternalServerError, "internal_error", "the transport failed to process the request", false)
				}
			}
		}()

		next.ServeHTTP(response, r)
	})
}

var requestSequence atomic.Uint64

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "req_" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("req_%x_%x", time.Now().UnixNano(), requestSequence.Add(1))
}

// recordingResponseWriter is a tiny wrapper that captures status +
// bytes so the response span can include them. Stays minimal —
// the body itself stays out of memory.
type recordingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *recordingResponseWriter) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recordingResponseWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

// Flush proxies through so SSE streams keep working.
func (r *recordingResponseWriter) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		if !r.wroteHeader {
			r.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

// Unwrap exposes the wrapped writer so http.ResponseController can reach the
// underlying connection — notably for the per-frame write deadline the SSE stream
// relies on to bound a blocked write (see serveStream).
func (r *recordingResponseWriter) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func handlerPanicError(recovered any) error {
	if cause, ok := recovered.(error); ok {
		return fmt.Errorf("http handler panicked: %w", cause)
	}
	return fmt.Errorf("http handler panicked: %v", recovered)
}
