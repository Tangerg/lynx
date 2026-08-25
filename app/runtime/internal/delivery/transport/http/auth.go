package http

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authGate enforces the local-token check on POST /v2/rpc. Two paths
// bypass: the operational sidecars and CORS preflights. Under
// streamable HTTP every stream is a POST, so the gate covers streaming
// too — there is no header-less EventSource to special-case (TRANSPORT
// §7/§11).
//
// On failure, the response is an RFC 9457 application/problem+json 401,
// not a JSON-RPC envelope, because authentication runs below the protocol
// layer (TRANSPORT §6.3).
func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.localToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions || isAuthBypassPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !validBearer(r.Header.Get("Authorization"), s.localToken) {
			// RFC 9110 §15.5.2 — a 401 MUST carry a challenge (TRANSPORT
			// §6.3/§11). The gate is a single bare Bearer scheme.
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "a valid local bearer token is required", true)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAuthBypassPath flags requests that intentionally skip the gate: the
// operational sidecars (no-auth ops endpoints). The RPC endpoint —
// including streaming POSTs — is gated.
func isAuthBypassPath(p string) bool {
	return isPublicEndpointPath(p)
}

// validBearer parses `Authorization: Bearer <token>` and compares
// the token in constant time. Returns false on missing header,
// wrong scheme, or token mismatch.
func validBearer(header, expected string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(header[len(prefix):])
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
