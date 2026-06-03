package http

import (
	"net/http"

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
// observability headers the FE reads (API.md §10); allowed headers are the
// transport-metadata set the FE sends (TRANSPORT §2). go-chi/cors answers
// preflight with 200 (the contract is silent on the exact 2xx — browsers
// accept either; the prior hand-rolled layer used 204).
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Last-Event-Id", "X-Conn-Id", "X-Protocol-Version", "X-Idempotency-Key", "X-Trace-Id"},
		ExposedHeaders:   []string{"X-Server", "X-Method", "X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           600,
	})
}
