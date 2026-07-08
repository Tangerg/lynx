package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	resp, err := headerHTTPClient("Bearer secret-token", nil).Do(req)
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
	client := headerHTTPClient("Bearer wins", map[string]string{
		"X-API-Key":     "k-123",
		"Authorization": "Bearer loses",
	})
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "k-123", apiKey)
	assert.Equal(t, "Bearer wins", auth)
}

func TestHeaderHTTPClientNilWhenEmpty(t *testing.T) {
	assert.Nil(t, headerHTTPClient("", nil))
}
