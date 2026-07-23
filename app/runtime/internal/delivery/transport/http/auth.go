package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalToken is the per-process gate token described in
// ../desktop/docs/protocol/TRANSPORT.md §11 (本地门禁 token). It only protects
// against other processes on the same machine — it is NOT user
// auth. The Web frontend reads the token from Path and sends
//
//	Authorization: Bearer <Value>
//
// on every POST /v2/rpc. The sidecars and the SSE stream
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
	switch p {
	case infoPath, livenessPath, readinessPath:
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
