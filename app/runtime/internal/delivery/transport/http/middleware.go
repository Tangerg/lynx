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

// observability wraps the mux with the entry-point tracing layer — the
// root of full-link tracing:
//
//   - extract any W3C traceparent the client sent, then open ONE server
//     span per request. Every downstream span (dispatch → engine → agent →
//     tools) hangs under it because the span rides r.Context() onward, so
//     a single trace covers the whole request, generated right here at the
//     entrance. The span carries http.* attributes + duration + body size,
//     and is marked Error on 5xx so backends can alert.
//   - panic recovery so the runtime survives a misbehaving handler; the panic
//     is recorded onto the request span, and an uncommitted response becomes a
//     flat 500 envelope without corrupting an already-started stream.
//
// All observability flows through OTel (see ../tracing.go for the shared
// package tracer); the global TracerProvider + propagator are wired once at
// process start (adapter/observability bootstrap).
func (s *Server) observability(next http.Handler) http.Handler {
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

		rec := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			span.SetAttributes(
				attribute.Int("http.response.status_code", rec.status),
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				attribute.Int("http.response.body.size", rec.bytes),
			)
			if rec.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(rec.status))
			}
			span.End()
		}()

		defer func() {
			if rcv := recover(); rcv != nil {
				err := handlerPanicError(rcv)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				if !rec.wroteHeader {
					writeProblem(rec, http.StatusInternalServerError, "internal_error", "the transport failed to process the request", false)
				}
			}
		}()

		next.ServeHTTP(rec, r)
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

func (w *recordingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// Flush proxies through so SSE streams keep working.
func (w *recordingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

func handlerPanicError(recovered any) error {
	if cause, ok := recovered.(error); ok {
		return fmt.Errorf("http handler panicked: %w", cause)
	}
	return fmt.Errorf("http handler panicked: %v", recovered)
}
