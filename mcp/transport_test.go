package mcp_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/mcp"
)

func TestServerConfig_Validate(t *testing.T) {
	cases := []struct {
		name string
		cfg  mcp.ServerConfig
		ok   bool
	}{
		{"http ok", mcp.ServerConfig{Name: "x", Transport: mcp.TransportHTTP, Endpoint: "https://e/"}, true},
		{"stdio ok", mcp.ServerConfig{Name: "x", Transport: mcp.TransportStdio, Command: "npx"}, true},
		{"missing name", mcp.ServerConfig{Transport: mcp.TransportHTTP, Endpoint: "https://e/"}, false},
		{"zero transport", mcp.ServerConfig{Name: "x", Endpoint: "https://e/"}, false},
		{"http without endpoint", mcp.ServerConfig{Name: "x", Transport: mcp.TransportHTTP}, false},
		{"http with command", mcp.ServerConfig{Name: "x", Transport: mcp.TransportHTTP, Endpoint: "https://e/", Command: "npx"}, false},
		{"stdio without command", mcp.ServerConfig{Name: "x", Transport: mcp.TransportStdio}, false},
		{"stdio with endpoint", mcp.ServerConfig{Name: "x", Transport: mcp.TransportStdio, Command: "npx", Endpoint: "https://e/"}, false},
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

func TestDial_ValidatesBeforeDialing(t *testing.T) {
	// An invalid config fails via Validate before any network / process work.
	_, err := mcp.Dial(context.Background(), nil,
		mcp.ServerConfig{Name: "x", Transport: mcp.TransportHTTP})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Endpoint is required")
}

func TestDial_NilClient(t *testing.T) {
	// A valid config with a nil client is rejected by the underlying dial.
	_, err := mcp.Dial(context.Background(), nil,
		mcp.ServerConfig{Name: "x", Transport: mcp.TransportHTTP, Endpoint: "https://e/"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client must not be nil")
}
