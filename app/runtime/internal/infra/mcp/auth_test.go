package mcp

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestDialStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want mcpserver.ConnectionState
	}{
		{"401 in message", errors.New("connect: server returned HTTP 401"), mcpserver.ConnectionNeedsAuth},
		{"unauthorized word", errors.New("initialize failed: Unauthorized"), mcpserver.ConnectionNeedsAuth},
		{"mixed case", errors.New("UNAUTHORIZED request"), mcpserver.ConnectionNeedsAuth},
		{"generic failure", errors.New("dial tcp: connection refused"), mcpserver.ConnectionFailed},
		{"403 is not needsAuth", errors.New("HTTP 403 Forbidden"), mcpserver.ConnectionFailed},
		{"nil", nil, mcpserver.ConnectionFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialStatus(tc.err); got != tc.want {
				t.Errorf("dialStatus(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
