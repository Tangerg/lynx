package a2a_test

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/Tangerg/lynx/a2a"
)

// echoAgent is a stub lynx Agent that streams a fixed reply, echoing the
// inbound text so the test can assert the message reached the server.
type echoAgent struct{}

func (echoAgent) Run(_ context.Context, input string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield("lynx received: "+input, nil)
	}
}

// TestRoundTrip wires the server side (echoAgent → NewHTTPHandler) behind an
// httptest server, then drives the client side (a2a.Tools → Tool.Call)
// against it — proving the full A2A loop: tool call → JSON-RPC → executor →
// task lifecycle → reply text, with the AgentCard resolved over the wire.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	// A mutable delegate lets us learn the server URL (needed for the card's
	// transport interface) before installing the real handler.
	var delegate http.Handler
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delegate.ServeHTTP(w, r)
	}))
	defer ts.Close()

	card := &sdka2a.AgentCard{
		Name:        "Echo Agent",
		Description: "Echoes the request back",
		SupportedInterfaces: []*sdka2a.AgentInterface{
			a2a.JSONRPCInterface(ts.URL + a2a.DefaultRPCPattern),
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Capabilities:       sdka2a.AgentCapabilities{Streaming: true},
		Skills: []sdka2a.AgentSkill{
			{ID: "echo", Name: "Echo", Description: "Echo the input", Tags: []string{"echo"}},
		},
	}

	handler, err := a2a.NewHTTPHandler(a2a.ServerConfig{Agent: echoAgent{}, Card: card})
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	delegate = handler

	// Client side: resolve the card and wrap the remote agent as a tool.
	tools, closeTools, err := a2a.Tools(ctx, a2a.Endpoint{CardURL: ts.URL})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	defer func() {
		if err := closeTools(); err != nil {
			t.Fatalf("close tools: %v", err)
		}
	}()

	if len(tools) != 1 {
		t.Fatalf("Tools returned %d tools, want 1", len(tools))
	}
	tool := tools[0]

	if got := tool.Definition().Name; got != "echo_agent" {
		t.Errorf("tool name = %q, want %q (sanitized from card name)", got, "echo_agent")
	}
	definition := tool.Definition()
	definition.InputSchema[0] = '['
	if got := tool.Definition().InputSchema[0]; got != '{' {
		t.Fatalf("mutating returned definition changed A2A tool schema prefix to %q", got)
	}

	out, err := tool.Call(ctx, `{"message":"hello"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "lynx received: hello") {
		t.Errorf("reply = %q, want it to contain the echoed request", out)
	}
}
