package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalToken is the per-process gate token described in
// docs/TRANSPORT.md §安全 + docs/API.md §鉴权. It only protects
// against other processes on the same machine — it is NOT user
// auth. The Web frontend reads the token from Path and sends
//
//	Authorization: Bearer <Value>
//
// on every POST /v1/rpc/{method}. The sidecars and the SSE stream
// stay open per the frontend's `httpTransport` contract.
type LocalToken struct {
	Value string
	Path  string
}

// IssueLocalToken generates a fresh 32-byte token, base64-encodes it,
// and writes it to path with mode 0600 (parent dir 0700). When path
// is empty it defaults to $HOME/.lyra/local-token.
func IssueLocalToken(path string) (*LocalToken, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("local token: locate home dir: %w", err)
		}
		path = filepath.Join(home, ".lyra", "local-token")
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("local token: read random: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("local token: mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return nil, fmt.Errorf("local token: write file: %w", err)
	}
	return &LocalToken{Value: value, Path: path}, nil
}

// authGate enforces the local-token check on POST /v1/rpc/*. Three
// paths bypass: sidecars (/v1/info, /v1/health), the SSE stream
// (EventSource can't set Authorization — withCredentials:false per
// TRANSPORT.md §4.3), and CORS preflights.
//
// On failure, the response is a flat-JSON 401 ({"error":
// "missing_local_token"}) — NOT the JSON-RPC envelope, since this
// fires below the protocol layer (API.md §7.3).
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
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAuthBypassPath flags requests that intentionally skip the gate:
// the two sidecars (no-auth ops endpoints) and the SSE stream
// (browser EventSource can't send Authorization).
func isAuthBypassPath(p string) bool {
	switch p {
	case "/v1/info", "/v1/health", "/v1/rpc/stream":
		return true
	}
	return false
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

// writeUnauthorized emits the exact body shape FE checks for (per
// API.md §7.3): a flat `{"error":"missing_local_token"}`.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: "missing_local_token"})
}
