package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  ServerConfig
		ok   bool
	}{
		{"http ok", ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e/"}, true},
		{"http relative endpoint", ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "/mcp"}, false},
		{"http non-http endpoint", ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "file:///tmp/mcp"}, false},
		{"stdio ok", ServerConfig{Name: "x", Transport: TransportStdio, Command: "npx"}, true},
		{"missing name", ServerConfig{Transport: TransportHTTP, Endpoint: "https://e/"}, false},
		{"zero transport", ServerConfig{Name: "x", Endpoint: "https://e/"}, false},
		{"http without endpoint", ServerConfig{Name: "x", Transport: TransportHTTP}, false},
		{"http with command", ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e/", Command: "npx"}, false},
		{"stdio without command", ServerConfig{Name: "x", Transport: TransportStdio}, false},
		{"stdio with endpoint", ServerConfig{Name: "x", Transport: TransportStdio, Command: "npx", Endpoint: "https://e/"}, false},
		{"http auth fields ok", ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e", Authorization: "Bearer t", Headers: map[string]string{"X-API-Key": "k"}}, true},
		{"stdio with auth", ServerConfig{Name: "x", Transport: TransportStdio, Command: "echo", Authorization: "Bearer t"}, false},
		{"stdio with headers", ServerConfig{Name: "x", Transport: TransportStdio, Command: "echo", Headers: map[string]string{"X-API-Key": "k"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestDialValidatesBeforeDialing(t *testing.T) {
	_, err := dial(context.Background(), nil,
		ServerConfig{Name: "x", Transport: TransportHTTP})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Endpoint is required")
}

func TestDialNilClient(t *testing.T) {
	_, err := dial(context.Background(), nil,
		ServerConfig{Name: "x", Transport: TransportHTTP, Endpoint: "https://e/"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client must not be nil")
}

func TestHeaderHTTPClientSetsAuthorization(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	client, err := endpointHTTPClient(srv.URL, "Bearer secret-token", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "Bearer secret-token", got)
	assert.Empty(t, req.Header.Get("Authorization"), "RoundTripper must not mutate caller request")
}

func TestHeaderHTTPClientSetsCustomHeaders(t *testing.T) {
	var apiKey, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey = r.Header.Get("X-API-Key")
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	client, err := endpointHTTPClient(srv.URL, "Bearer wins", map[string]string{
		"X-API-Key":     "k-123",
		"Authorization": "Bearer loses",
	})
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "k-123", apiKey)
	assert.Equal(t, "Bearer wins", auth)
}

func TestEndpointHTTPClientEnforcesOriginWithoutHeaders(t *testing.T) {
	client, err := endpointHTTPClient("https://example.com/mcp", "", nil)
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestEndpointHTTPClientFollowsSameOriginRedirect(t *testing.T) {
	var gotAuthorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/mcp", http.StatusFound)
			return
		}
		gotAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := endpointHTTPClient(srv.URL+"/start", "Bearer secret-token", nil)
	require.NoError(t, err)
	resp, err := client.Get(srv.URL + "/start")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, "Bearer secret-token", gotAuthorization)
}

func TestEndpointHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()

	client, err := endpointHTTPClient(source.URL, "Bearer secret-token", map[string]string{"X-API-Key": "secret"})
	require.NoError(t, err)
	resp, err := client.Get(source.URL)
	require.Error(t, err)
	assert.ErrorIs(t, err, errCrossOrigin)
	assert.False(t, targetHit.Load(), "cross-origin redirect reached target")
	if resp != nil {
		_ = resp.Body.Close()
	}
}

func TestHeaderRoundTripperRejectsUnboundTarget(t *testing.T) {
	client, err := endpointHTTPClient("https://trusted.example/mcp", "Bearer secret-token", nil)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "https://other.example/mcp", nil)
	require.NoError(t, err)
	_, err = client.Do(req)
	if !errors.Is(err, errCrossOrigin) {
		t.Fatalf("Do error = %v, want errCrossOrigin", err)
	}
}
