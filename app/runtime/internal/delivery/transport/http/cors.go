package http

import (
	"net/http"
	"slices"

	"github.com/go-chi/cors"
)

// DefaultCORSOrigins returns the allowlist baked in for the runtime HTTP server
// when the operator hasn't supplied CORS origins. It covers the browser shells
// and development servers actually shipped with the application. Each caller
// owns its result, so server configuration cannot mutate the package default.
func DefaultCORSOrigins() []string {
	return []string{
		"wails://localhost",      // Wails macOS/Linux asset server
		"http://wails.localhost", // Wails Windows asset server
		"http://127.0.0.1:5174",  // repository Wails/Vite dev task
		"http://localhost:5173",  // standalone Vite default
	}
}

// corsMiddleware builds the CORS layer from the origin allowlist. An empty
// list means "no CORS" (same-origin only) — a pass-through. go-chi/cors
// owns the spec mechanics (origin match incl. "*", preflight, Vary,
// credentials); we only declare the policy. Exposed headers are the three
// observability headers browser clients read; allowed headers are the transport
// metadata they send. go-chi/cors answers
// preflight with 200 (the contract is silent on the exact 2xx — browsers
// accept either; the prior hand-rolled layer used 204).
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	opts := cors.Options{
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		// Transport metadata sent by browser clients plus the W3C trace-context
		// headers their OTel layer injects on every request
		// (traceparent / tracestate / baggage) so the client span extends the
		// backend trace. Trace correlation is W3C-only — no X-Trace-Id.
		// Omitting the trace headers fails the preflight for EVERY method,
		// since browser tracing injects traceparent unconditionally.
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", "Idempotency-Namespace", "Last-Event-Id", "traceparent", "tracestate", "baggage"},
		ExposedHeaders:   []string{"Request-Id", "X-Server", "X-Method"},
		AllowCredentials: true,
		MaxAge:           600,
	}
	// "*" means allow every origin. With AllowCredentials the CORS spec
	// forbids a literal "*" in Access-Control-Allow-Origin (browsers reject
	// it on a credentialed request), so REFLECT the request origin instead —
	// a credentials-compatible allow-all — rather than emitting "*". This is
	// reserved for an operator's explicit development configuration; the
	// built-in Wails origins use the exact allowlist above. An explicit
	// allowlist passes through unchanged.
	if slices.Contains(origins, "*") {
		opts.AllowOriginFunc = func(*http.Request, string) bool { return true }
	} else {
		opts.AllowedOrigins = origins
	}
	return cors.Handler(opts)
}
