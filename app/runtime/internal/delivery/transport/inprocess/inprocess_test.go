package inprocess_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport/inprocess"
)

// fakeRuntime is just enough Runtime to drive the InProcess transport's
// dispatch + response paths. Methods that aren't exercised by the
// test embed the interface so the type satisfies protocol.Runtime
// without us having to stub all entries.
type fakeRuntime struct{ protocol.Runtime }

func (fakeRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Protocol: protocol.SupportedProtocolRange()}, nil
}

// TestInProcessRoundtrip confirms a Request sent to the InProcess
// transport surfaces as a Response on the Recv channel — proves the
// dispatcher + transport wiring is correctly bidirectional.
func TestInProcessRoundtrip(t *testing.T) {
	tp, err := inprocess.NewTransport(inprocess.Config{Runtime: fakeRuntime{}})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	defer tp.Close()

	req, err := transport.NewCall("1", "runtime.discover", map[string]any{})
	if err != nil {
		t.Fatalf("NewCall: %v", err)
	}
	if err := tp.Send(context.Background(), req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg := <-tp.Recv():
		resp, ok := msg.(*transport.Response)
		if !ok {
			t.Fatalf("expected *Response, got %T", msg)
		}
		if resp.Error != nil {
			t.Fatalf("got error envelope: %+v", resp.Error)
		}
		if !strings.Contains(string(resp.Result), "2026-07-19") {
			t.Fatalf("missing protocolVersion in result: %s", string(resp.Result))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestInProcessUnknownMethod confirms unknown methods get -32601 +
// the dispatcher's standard envelope. Covers the failure path.
func TestInProcessUnknownMethod(t *testing.T) {
	tp, err := inprocess.NewTransport(inprocess.Config{Runtime: fakeRuntime{}})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	defer tp.Close()

	// A method the fakeRuntime doesn't declare falls through to
	// the dispatcher's default branch.
	bogus, _ := transport.NewCall("1", "totally.bogus", nil)
	_ = tp.Send(context.Background(), bogus)

	select {
	case msg := <-tp.Recv():
		resp, ok := msg.(*transport.Response)
		if !ok {
			t.Fatalf("expected *Response, got %T", msg)
		}
		if resp.Error == nil {
			t.Fatal("expected error envelope")
		}
		rpcErr, ok := resp.Error.(*transport.Error)
		if !ok {
			t.Fatalf("Error is %T, want *transport.Error", resp.Error)
		}
		if rpcErr.Code != transport.CodeMethodNotFound {
			t.Fatalf("error.code = %d, want %d", rpcErr.Code, transport.CodeMethodNotFound)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

type blockingRuntime struct {
	protocol.Runtime
	started chan struct{}
	stopped chan struct{}
}

func (r *blockingRuntime) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	close(r.started)
	<-ctx.Done()
	close(r.stopped)
	return nil, ctx.Err()
}

func TestInProcessCloseCancelsAndJoinsActiveCall(t *testing.T) {
	api := &blockingRuntime{started: make(chan struct{}), stopped: make(chan struct{})}
	tp, err := inprocess.NewTransport(inprocess.Config{Runtime: api})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	req, err := transport.NewCall("1", "runtime.discover", nil)
	if err != nil {
		t.Fatalf("NewCall: %v", err)
	}
	sent := make(chan error, 1)
	go func() { sent <- tp.Send(context.Background(), req) }()

	select {
	case <-api.started:
	case <-time.After(time.Second):
		t.Fatal("dispatcher call did not start")
	}
	if err := tp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-api.stopped:
	default:
		t.Fatal("Close returned before the active call observed cancellation")
	}
	select {
	case err := <-sent:
		if err == nil {
			t.Fatal("Send succeeded after Close began")
		}
	case <-time.After(time.Second):
		t.Fatal("Send outlived Close")
	}
}

type streamingRuntime struct {
	protocol.Runtime
	started  chan struct{}
	canceled chan struct{}
}

func (r *streamingRuntime) WorkspaceSubscribe(
	ctx context.Context,
	_ protocol.WorkspaceSubscribeRequest,
) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	events := make(chan protocol.WorkspaceEvent)
	close(r.started)
	context.AfterFunc(ctx, func() {
		close(r.canceled)
		close(events)
	})
	return &protocol.WorkspaceSubscribeResponse{}, events, nil
}

func TestInProcessStreamingCallLivesUntilTransportClose(t *testing.T) {
	api := &streamingRuntime{started: make(chan struct{}), canceled: make(chan struct{})}
	tp, err := inprocess.NewTransport(inprocess.Config{Runtime: api})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	req, err := transport.NewCall("1", "workspace.subscribe", map[string]any{})
	if err != nil {
		t.Fatalf("NewCall: %v", err)
	}
	if err := tp.Send(context.Background(), req); err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-api.started
	select {
	case <-api.canceled:
		t.Fatal("stream context was canceled when Send returned")
	case <-time.After(20 * time.Millisecond):
	}

	if err := tp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-api.canceled:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel the streaming call context")
	}
}
