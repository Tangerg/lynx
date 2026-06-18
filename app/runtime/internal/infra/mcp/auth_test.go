package mcp

import (
	"errors"
	"testing"
)

func TestDialStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401 in message", errors.New("connect: server returned HTTP 401"), statusNeedsAuth},
		{"unauthorized word", errors.New("initialize failed: Unauthorized"), statusNeedsAuth},
		{"mixed case", errors.New("UNAUTHORIZED request"), statusNeedsAuth},
		{"generic failure", errors.New("dial tcp: connection refused"), statusFailed},
		{"403 is not needsAuth", errors.New("HTTP 403 Forbidden"), statusFailed},
		{"nil", nil, statusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialStatus(tc.err); got != tc.want {
				t.Errorf("dialStatus(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
