package a2a_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

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

func TestNewHTTPHandlerRequiresAgentAndCard(t *testing.T) {
	card := &sdka2a.AgentCard{Name: "test"}
	var nilAgent *echoAgent
	tests := []struct {
		name string
		cfg  a2a.ServerConfig
		want error
	}{
		{
			name: "agent",
			cfg:  a2a.ServerConfig{Card: card},
			want: a2a.ErrNilAgent,
		},
		{
			name: "typed nil agent",
			cfg:  a2a.ServerConfig{Agent: nilAgent, Card: card},
			want: a2a.ErrNilAgent,
		},
		{
			name: "card",
			cfg:  a2a.ServerConfig{Agent: echoAgent{}},
			want: a2a.ErrNilCard,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := a2a.NewHTTPHandler(test.cfg); !errors.Is(err, test.want) {
				t.Fatalf("NewHTTPHandler error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewHTTPHandlerRejectsInvalidCardAndPattern(t *testing.T) {
	tests := []struct {
		name string
		cfg  a2a.ServerConfig
		want error
	}{
		{
			name: "card cannot be encoded",
			cfg: a2a.ServerConfig{Agent: echoAgent{}, Card: &sdka2a.AgentCard{
				Name: "invalid",
				Signatures: []sdka2a.AgentCardSignature{{
					Header: map[string]any{"unsupported": func() {}},
				}},
			}},
			want: a2a.ErrInvalidCard,
		},
		{
			name: "malformed RPC pattern",
			cfg:  a2a.ServerConfig{Agent: echoAgent{}, Card: &sdka2a.AgentCard{Name: "test"}, RPCPattern: "/{"},
			want: a2a.ErrInvalidRPCPattern,
		},
		{
			name: "RPC pattern conflicts with card endpoint",
			cfg: a2a.ServerConfig{
				Agent: echoAgent{}, Card: &sdka2a.AgentCard{Name: "test"},
				RPCPattern: a2asrv.WellKnownAgentCardPath,
			},
			want: a2a.ErrInvalidRPCPattern,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := a2a.NewHTTPHandler(test.cfg); !errors.Is(err, test.want) {
				t.Fatalf("NewHTTPHandler error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewHTTPHandlerSnapshotsAgentCard(t *testing.T) {
	card := &sdka2a.AgentCard{
		Name: "original",
		Skills: []sdka2a.AgentSkill{{
			ID: "read", Name: "Read",
		}},
	}
	handler, err := a2a.NewHTTPHandler(a2a.ServerConfig{Agent: echoAgent{}, Card: card})
	if err != nil {
		t.Fatal(err)
	}
	card.Name = "mutated"
	card.Skills[0].Name = "Mutated"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, a2asrv.WellKnownAgentCardPath, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("AgentCard status = %d, want 200", recorder.Code)
	}
	var served sdka2a.AgentCard
	if err := json.Unmarshal(recorder.Body.Bytes(), &served); err != nil {
		t.Fatalf("decode served AgentCard: %v", err)
	}
	if served.Name != "original" || len(served.Skills) != 1 || served.Skills[0].Name != "Read" {
		t.Fatalf("served AgentCard = %#v, want construction snapshot", served)
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

	for name, arguments := range map[string]string{
		"bare string":        `"hello"`,
		"missing message":    `{}`,
		"empty message":      `{"message":" "}`,
		"unknown property":   `{"message":"hello","extra":true}`,
		"multiple objects":   `{"message":"hello"} {}`,
		"malformed argument": `{"message":`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tool.Call(ctx, arguments); err == nil {
				t.Fatal("Call succeeded for arguments outside the declared object schema")
			}
		})
	}
}
