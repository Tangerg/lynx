package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestDialStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want mcpserver.ConnectionState
	}{
		{"typed auth rejection", &dialFailure{kind: dialFailureNeedsAuth, err: errors.New("connect rejected")}, mcpserver.ConnectionNeedsAuth},
		{"wrapped auth rejection", errors.Join(errors.New("dial failed"), &dialFailure{kind: dialFailureNeedsAuth, err: errors.New("connect rejected")}), mcpserver.ConnectionNeedsAuth},
		{"401 text is not a type", errors.New("connect: server returned HTTP 401"), mcpserver.ConnectionFailed},
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

func TestHTTPTransportClassifiesObservedUnauthorizedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := endpointHTTPClient(server.URL, "", nil)
	if err != nil {
		t.Fatalf("endpointHTTPClient: %v", err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response: %v", err)
	}

	dialErr := classifyHTTPDialError(client, errors.New("SDK discarded response status"))
	if got := dialStatus(dialErr); got != mcpserver.ConnectionNeedsAuth {
		t.Fatalf("dial status = %q, want needsAuth", got)
	}
}
