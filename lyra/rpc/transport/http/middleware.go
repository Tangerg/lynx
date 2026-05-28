package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// observability wraps the mux with a single middleware layer that
// implements the API.md §10 contract:
//
//   - one OTel span per 4xx/5xx response with path / http_method /
//     status / duration_ms / bytes_out / trace_id attributes —
//     2xx stays quiet so dev consoles aren't flooded
//   - panic recovery with a flat 500 envelope so the runtime
//     survives a misbehaving handler; panic is recorded onto a
//     short-lived error span before the response goes out
//
// All observability flows through OTel (see ../tracing.go for the
// shared package tracer); stdlib log is not used here per the
// project-level "logging goes through OTel" rule.
func (s *Server) observability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec.status >= 400 {
				recordResponse(r, rec.status, time.Since(start), rec.bytes)
			}
		}()

		defer func() {
			if rcv := recover(); rcv != nil {
				recordError("rpc.panic",
					fmt.Errorf("%v", rcv),
					attribute.String("http.target", r.URL.Path),
					attribute.String("http.method", r.Method),
				)
				writeTransportError(w, r, http.StatusInternalServerError, "internal error")
			}
		}()

		next.ServeHTTP(rec, r)
	})
}

// recordResponse emits a short-lived OTel span carrying the
// structured fields the old stdlib access log carried. 5xx
// responses are marked Error so backends can alert on them;
// 4xx is left as Ok status (client problem, not server fault).
func recordResponse(r *http.Request, status int, duration time.Duration, bytes int) {
	_, span := tracer.Start(context.Background(), "rpc.response",
		trace.WithAttributes(
			attribute.String("http.target", r.URL.Path),
			attribute.String("http.method", r.Method),
			attribute.Int("http.status_code", status),
			attribute.Int64("duration_ms", duration.Milliseconds()),
			attribute.Int("http.response.body.size", bytes),
			attribute.String("lynx.lyra.trace_id", r.Header.Get("X-Lyra-Trace-Id")),
		),
	)
	if status >= 500 {
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	span.End()
}

// recordingResponseWriter is a tiny wrapper that captures status +
// bytes so the response span can include them. Stays minimal —
// the body itself stays out of memory.
type recordingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *recordingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// Flush proxies through so SSE streams keep working.
func (w *recordingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
