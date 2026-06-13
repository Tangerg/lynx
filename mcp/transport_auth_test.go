package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthHTTPClient_SetsHeader verifies the Authorization header reaches the
// server and that the caller's request is left unmutated (RoundTripper contract).
func TestAuthHTTPClient_SetsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := authHTTPClient("Bearer secret-token").Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	if got != "Bearer secret-token" {
		t.Errorf("server saw Authorization = %q, want %q", got, "Bearer secret-token")
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("authRoundTripper mutated the caller's request header")
	}
}

// TestServerConfig_Validate_AuthorizationHTTPOnly: Authorization is valid on
// HTTP transport and rejected on stdio (a subprocess authenticates via Env).
func TestServerConfig_Validate_AuthorizationHTTPOnly(t *testing.T) {
	if err := (ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e", Authorization: "Bearer t"}).Validate(); err != nil {
		t.Errorf("HTTP + Authorization should validate, got %v", err)
	}
	if err := (ServerConfig{Name: "x", Transport: TransportStdio, Command: "echo", Authorization: "Bearer t"}).Validate(); err == nil {
		t.Error("Authorization on stdio transport should be rejected")
	}
}
