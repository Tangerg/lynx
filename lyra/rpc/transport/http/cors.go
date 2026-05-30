package http

import (
	"net/http"
	"strings"
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

// corsConfig is an immutable view of the per-server CORS policy.
// An empty Origins list means "no CORS" — pure same-origin only.
type corsConfig struct {
	origins []string
}

// allows is exact-match; "*" anywhere in the list short-circuits to
// "wildcard". We only echo the wildcard back when credentials are
// not in play (which they are, here — Authorization), so "*" is
// mostly a dev-mode escape hatch.
func (c corsConfig) allows(origin string) (matched bool, wildcard bool) {
	for _, o := range c.origins {
		if o == "*" {
			return true, true
		}
		if o == origin {
			return true, false
		}
	}
	return false, false
}

// cors wraps next with CORS headers + OPTIONS preflight handling.
// Same-origin requests (no Origin header) pass through untouched —
// curl / loopback ops use cases stay flat.
//
// Headers exposed to the browser are the three observability headers
// the FE needs to surface (X-Lyra-Method / X-Lyra-Request-Id /
// X-Lyra-Server) — see API.md §10.
func (s *Server) cors(next http.Handler) http.Handler {
	cfg := s.corsCfg
	if len(cfg.origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		matched, wildcard := cfg.allows(origin)
		if !matched {
			// Disallowed origin — browser will block the response.
			// Still forward so the request itself can be observed.
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		if wildcard {
			h.Set("Access-Control-Allow-Origin", "*")
		} else {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		h.Set("Access-Control-Expose-Headers", corsExposedHeaders)

		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Headers we let the browser send / read. Kept as joined strings to
// avoid per-request allocation in the hot path.
var (
	corsAllowedHeaders = strings.Join([]string{
		"Authorization",
		"Content-Type",
		"Last-Event-Id",
		"Lyra-Connection-Id",    // §1.2 — sent on every POST; preflight must allow it
		"Lyra-Protocol-Version", // §1.2 out-of-band metadata
		"X-Lyra-Trace-Id",
	}, ", ")

	corsExposedHeaders = strings.Join([]string{
		"X-Lyra-Server",
		"X-Lyra-Method",
		"X-Lyra-Request-Id",
	}, ", ")
)
