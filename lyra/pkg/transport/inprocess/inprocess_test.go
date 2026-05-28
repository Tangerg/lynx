package inprocess_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
	"github.com/Tangerg/lynx/lyra/pkg/transport/inprocess"
)

// fakeAPI is just enough CoreAPI to drive the InProcess transport's
// dispatch + response paths. Methods that aren't exercised by the
// test embed the interface so the type satisfies coreapi.CoreAPI
// without us having to stub all 32 entries.
type fakeAPI struct{ coreapi.CoreAPI }

func (fakeAPI) Initialize(_ context.Context, _ coreapi.InitializeIn) (*coreapi.InitializeOut, error) {
	return &coreapi.InitializeOut{ProtocolVersion: "2026-05-28"}, nil
}

func (fakeAPI) Ping(_ context.Context) error { return nil }

// TestInProcessRoundtrip confirms a Request sent to the InProcess
// transport surfaces as a Response on the Recv channel — proves the
// dispatcher + transport wiring is correctly bidirectional.
func TestInProcessRoundtrip(t *testing.T) {
	tp, err := inprocess.NewTransport(inprocess.Config{API: fakeAPI{}})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	defer tp.Close()

	req := &transport.Message{
		JSONRPC: transport.JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  "runtime.initialize",
		Params:  json.RawMessage(`{}`),
	}
	if err := tp.Send(context.Background(), req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-tp.Recv():
		if msg.Error != nil {
			t.Fatalf("got error envelope: %+v", msg.Error)
		}
		if !contains(msg.Result, "2026-05-28") {
			t.Fatalf("missing protocolVersion in result: %s", string(msg.Result))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestInProcessUnknownMethod confirms unknown methods get -32601 +
// the dispatcher's standard envelope. Covers the failure path.
func TestInProcessUnknownMethod(t *testing.T) {
	tp, err := inprocess.NewTransport(inprocess.Config{API: fakeAPI{}})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	defer tp.Close()

	// Initialize first so the gate doesn't fire.
	_ = tp.Send(context.Background(), &transport.Message{
		JSONRPC: transport.JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  "runtime.initialize",
	})
	<-tp.Recv()

	// Now a method the fakeAPI doesn't declare — falls through to
	// the dispatcher's default branch.
	bogus := &transport.Message{
		JSONRPC: transport.JSONRPCVersion,
		ID:      json.RawMessage(`2`),
		Method:  "totally.bogus",
	}
	_ = tp.Send(context.Background(), bogus)

	select {
	case msg := <-tp.Recv():
		if msg.Error == nil {
			t.Fatal("expected error envelope")
		}
		if msg.Error.Code != transport.CodeMethodNotFound {
			t.Fatalf("error.code = %d, want %d", msg.Error.Code, transport.CodeMethodNotFound)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func contains(raw json.RawMessage, needle string) bool {
	return len(raw) > 0 && bytesContains(string(raw), needle)
}

// bytesContains avoids importing strings to keep test deps tight.
func bytesContains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
