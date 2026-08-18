package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalToken is the transport's data-directory gate token. It only protects
// the loopback transport — it is NOT user authentication. A trusted local client
// reads the token from Path and sends
//
//	Authorization: Bearer <Value>
//
// on every POST /v2/rpc. The sidecars and SSE streams remain unauthenticated by
// this token.
type LocalToken struct {
	Value string
	Path  string
}

// OpenLocalToken loads the token owned by path, creating a random 32-byte value
// with mode 0600 when the path has no token. Runtime process replacement keeps
// the same value so a live local client can authenticate the successor without
// acquiring a second credential lifecycle. The executable supplies the durable
// data path; Transport never discovers host directories.
func OpenLocalToken(path string) (*LocalToken, error) {
	if path == "" {
		return nil, errors.New("local token: path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("local token: path must be absolute")
	}

	token, err := readLocalToken(path)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("local token: mkdir: %w", err)
	}
	value, err := newLocalTokenValue()
	if err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".local-token-*")
	if err != nil {
		return nil, fmt.Errorf("local token: create candidate: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(value); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("local token: write candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("local token: sync candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("local token: close candidate: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return readLocalToken(path)
		}
		return nil, fmt.Errorf("local token: publish candidate: %w", err)
	}
	return &LocalToken{Value: value, Path: path}, nil
}

func newLocalTokenValue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("local token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func readLocalToken(path string) (*LocalToken, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("local token: path must be a regular file")
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		return nil, fmt.Errorf("local token: permissions are %04o, want 0600", permissions)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("local token: read file: %w", err)
	}
	value := strings.TrimSpace(string(data))
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("local token: file does not contain one 32-byte token")
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
