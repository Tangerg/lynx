package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type observedResponse struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (response *observedResponse) WriteHeader(status int) {
	if response.status != 0 {
		return
	}
	response.status = status
	response.ResponseWriter.WriteHeader(status)
}

func (response *observedResponse) Write(payload []byte) (int, error) {
	if response.status == 0 {
		response.WriteHeader(http.StatusOK)
	}
	written, err := response.ResponseWriter.Write(payload)
	response.bytes += int64(written)
	return written, err
}

func (response *observedResponse) Unwrap() http.ResponseWriter {
	return response.ResponseWriter
}

func (server *Server) withTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		ctx := server.propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		observed := &observedResponse{ResponseWriter: response}
		next.ServeHTTP(observed, request.WithContext(ctx))
		if observed.status == 0 {
			observed.status = http.StatusOK
		}
		attributes := []slog.Attr{
			slog.String("request_id", response.Header().Get("Request-Id")),
			slog.String("http_method", request.Method),
			slog.String("http_path", request.URL.Path),
			slog.Int("http_status", observed.status),
			slog.Int64("response_bytes", observed.bytes),
			slog.Duration("duration", time.Since(started)),
		}
		span := trace.SpanContextFromContext(ctx)
		if span.IsValid() {
			attributes = append(attributes,
				slog.String("trace_id", span.TraceID().String()),
				slog.String("parent_span_id", span.SpanID().String()),
				slog.String("trace_flags", span.TraceFlags().String()),
			)
		}
		server.logger.LogAttrs(ctx, slog.LevelInfo, "HTTP request completed", attributes...)
	})
}
