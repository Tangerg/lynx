package http

import (
	"net/http"
	"slices"

	"github.com/go-chi/cors"
)

// DefaultCORSOrigins is the allowlist baked in for `lyra serve` when
// the operator hasn't supplied --cors-origin. Covers the Web frontend
// shells we ship with + the dev servers we use day to day.
var DefaultCORSOrigins = []string{
	"tauri://localhost",
	"http://tauri.localhost",
	"http://localhost:1420", // Tauri dev
	"http://localhost:5173", // Vite default
	"http://localhost:3000", // Next.js / CRA
}

// corsMiddleware builds the CORS layer from the origin allowlist. An empty
// list means "no CORS" (same-origin only) — a pass-through. go-chi/cors
// owns the spec mechanics (origin match incl. "*", preflight, Vary,
// credentials); we only declare the policy. Exposed headers are the three
// observability headers the FE reads (TRANSPORT §13); allowed headers are
// the transport-metadata set the FE sends (TRANSPORT §2). go-chi/cors answers
// preflight with 200 (the contract is silent on the exact 2xx — browsers
// accept either; the prior hand-rolled layer used 204).
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	opts := cors.Options{
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		// Transport-metadata headers the FE sends (TRANSPORT §2) + the W3C
		// trace-context headers its OTel layer injects on every request
		// (traceparent / tracestate / baggage) so the FE span extends the
		// backend trace. Trace correlation is W3C-only — no X-Trace-Id.
		// Omitting the trace headers fails the preflight for EVERY method,
		// since the FE injects traceparent unconditionally.
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Last-Event-Id", "X-Protocol-Version", "X-Idempotency-Key", "traceparent", "tracestate", "baggage"},
		ExposedHeaders:   []string{"X-Server", "X-Method"},
		AllowCredentials: true,
		MaxAge:           600,
	}
	// "*" means allow every origin. With AllowCredentials the CORS spec
	// forbids a literal "*" in Access-Control-Allow-Origin (browsers reject
	// it on a credentialed request), so REFLECT the request origin instead —
	// a credentials-compatible allow-all — rather than emitting "*". This is
	// what makes the Wails webview origin (wails://wails.localhost) work
	// while the config keeps the simple "*" dev default. An explicit
	// allowlist passes through unchanged.
	if slices.Contains(origins, "*") {
		opts.AllowOriginFunc = func(*http.Request, string) bool { return true }
	} else {
		opts.AllowedOrigins = origins
	}
	return cors.Handler(opts)
}
