package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

func TestEndpointRemainsComparable(t *testing.T) {
	endpoint := Endpoint{CardURL: "https://agent.example"}
	set := map[Endpoint]struct{}{endpoint: {}}
	if _, ok := set[endpoint]; !ok {
		t.Fatal("Endpoint map lookup failed")
	}
}

func TestDialBoundsAgentCardResolution(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	_, _, err := dial(t.Context(), server.URL, dialOptions{CardTimeout: 20 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dial error = %v, want context deadline exceeded", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("Agent Card request did not start")
	}
}

func TestDialRejectsInvalidCardConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		cardURL string
		opts    dialOptions
		want    error
	}{
		{name: "relative card URL", cardURL: "/agent", want: ErrInvalidCardURL},
		{name: "non-HTTP card URL", cardURL: "file:///tmp/card.json", want: ErrInvalidCardURL},
		{name: "negative timeout", cardURL: "https://agent.example", opts: dialOptions{CardTimeout: -time.Second}, want: ErrInvalidCardTimeout},
		{name: "RPC origin with path", cardURL: "https://agent.example", opts: dialOptions{AllowedRPCOrigins: []string{"https://rpc.example/path"}}, want: ErrInvalidRPCOrigin},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := dial(t.Context(), test.cardURL, test.opts)
			if !errors.Is(err, test.want) {
				t.Fatalf("dial error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDialRejectsCrossOriginAgentCardRedirect(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(source.Close)

	_, _, err := dial(t.Context(), source.URL, dialOptions{})
	if !errors.Is(err, ErrOriginNotAllowed) {
		t.Fatalf("dial error = %v, want ErrOriginNotAllowed", err)
	}
	if targetHit.Load() {
		t.Fatal("cross-origin Agent Card redirect reached target")
	}
}

func TestDialRequiresExplicitTrustForCrossOriginRPC(t *testing.T) {
	rpc := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(rpc.Close)
	card := testAgentCard(rpc.URL)
	cardServer := serveAgentCard(t, card)

	_, _, err := dial(t.Context(), cardServer.URL, dialOptions{})
	if !errors.Is(err, ErrOriginNotAllowed) {
		t.Fatalf("strict dial error = %v, want ErrOriginNotAllowed", err)
	}

	client, _, err := dial(t.Context(), cardServer.URL, dialOptions{AllowedRPCOrigins: []string{rpc.URL}})
	if err != nil {
		t.Fatalf("explicitly trusted dial: %v", err)
	}
	if err := client.Destroy(); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}

func TestRestrictedHTTPClientDoesNotMutateCallerClient(t *testing.T) {
	var redirectCalls atomic.Int32
	base := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		redirectCalls.Add(1)
		return nil
	}}
	policy, err := newEndpointOriginPolicy("https://agent.example", nil)
	if err != nil {
		t.Fatalf("newEndpointOriginPolicy: %v", err)
	}
	restricted := restrictedHTTPClient(base, policy.cardOrigins)
	if restricted == base {
		t.Fatal("restrictedHTTPClient returned caller-owned client")
	}
	if base.Transport != nil || base.CheckRedirect == nil {
		t.Fatal("restrictedHTTPClient mutated caller-owned client")
	}
	req, err := http.NewRequest(http.MethodGet, "https://other.example/card", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := restricted.Do(req); !errors.Is(err, ErrOriginNotAllowed) {
		t.Fatalf("Do error = %v, want ErrOriginNotAllowed", err)
	}
	if redirectCalls.Load() != 0 {
		t.Fatalf("caller redirect policy invoked %d time(s) for blocked initial request", redirectCalls.Load())
	}
}

func testAgentCard(rpcURL string) *sdka2a.AgentCard {
	return &sdka2a.AgentCard{
		Name: "test",
		SupportedInterfaces: []*sdka2a.AgentInterface{
			sdka2a.NewAgentInterface(rpcURL, sdka2a.TransportProtocolJSONRPC),
		},
	}
}

func serveAgentCard(t *testing.T, card *sdka2a.AgentCard) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(card); err != nil {
			t.Errorf("encode Agent Card: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
