package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHeaderHTTPClient_SetsAuthorization verifies the Authorization header
// reaches the server and that the caller's request is left unmutated
// (RoundTripper contract).
func TestHeaderHTTPClient_SetsAuthorization(t *testing.T) {
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
	resp, err := headerHTTPClient("Bearer secret-token", nil).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	if got != "Bearer secret-token" {
		t.Errorf("server saw Authorization = %q, want %q", got, "Bearer secret-token")
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("headerRoundTripper mutated the caller's request header")
	}
}

// TestHeaderHTTPClient_SetsCustomHeaders verifies arbitrary headers reach the
// server, and that the dedicated authorization wins over a headers
// "Authorization" entry (the bearer field is authoritative).
func TestHeaderHTTPClient_SetsCustomHeaders(t *testing.T) {
	var apiKey, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("X-API-Key")
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	client := headerHTTPClient("Bearer wins", map[string]string{
		"X-API-Key":     "k-123",
		"Authorization": "Bearer loses", // overridden by the dedicated field
	})
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	if apiKey != "k-123" {
		t.Errorf("server saw X-API-Key = %q, want k-123", apiKey)
	}
	if auth != "Bearer wins" {
		t.Errorf("server saw Authorization = %q, want the dedicated bearer to win", auth)
	}
}

// TestHeaderHTTPClient_NilWhenEmpty: no authorization and no headers means no
// custom client (callers then use the SDK default).
func TestHeaderHTTPClient_NilWhenEmpty(t *testing.T) {
	if c := headerHTTPClient("", nil); c != nil {
		t.Errorf("headerHTTPClient(empty) = %v, want nil", c)
	}
}

// TestServerConfig_Validate_HTTPOnlyFields: Authorization and Headers are valid
// on HTTP transport and rejected on stdio (a subprocess has no request headers
// and authenticates via Env).
func TestServerConfig_Validate_HTTPOnlyFields(t *testing.T) {
	if err := (ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e", Authorization: "Bearer t", Headers: map[string]string{"X-API-Key": "k"}}).Validate(); err != nil {
		t.Errorf("HTTP + Authorization + Headers should validate, got %v", err)
	}
	if err := (ServerConfig{Name: "x", Transport: TransportStdio, Command: "echo", Authorization: "Bearer t"}).Validate(); err == nil {
		t.Error("Authorization on stdio transport should be rejected")
	}
	if err := (ServerConfig{Name: "x", Transport: TransportStdio, Command: "echo", Headers: map[string]string{"X-API-Key": "k"}}).Validate(); err == nil {
		t.Error("Headers on stdio transport should be rejected")
	}
}
