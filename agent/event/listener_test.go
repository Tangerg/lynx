package event_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

var listenerDeployment = core.DeploymentRef{Name: "x", Digest: "digest"}

func TestNamedListener_NameAndOnEvent(t *testing.T) {
	var got []event.Kind
	listener := event.NewNamedListener("collector", func(_ context.Context, e event.Event) {
		got = append(got, e.Kind())
	})

	if listener.Name() != "collector" {
		t.Fatalf("Name() = %q, want %q", listener.Name(), "collector")
	}

	mc := event.NewMulticast()
	mc.Add(listener)
	mc.OnEvent(context.Background(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
	mc.OnEvent(context.Background(), event.AgentUndeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})

	if len(got) != 2 {
		t.Fatalf("captured %d events, want 2: %v", len(got), got)
	}
	if got[0] != "agent_deployed" || got[1] != "agent_undeployed" {
		t.Fatalf("captured order = %v", got)
	}
}

func TestNamedListener_NilFnIsNop(t *testing.T) {
	listener := event.NewNamedListener("nop", nil)

	// Should not panic.
	listener.OnEvent(context.Background(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
}

func TestMulticastCancelListenerFunc(t *testing.T) {
	var calls int
	multicast := event.NewMulticast()
	cancel := multicast.Add(event.ListenerFunc(func(context.Context, event.Event) {
		calls++
	}))

	multicast.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
	cancel()
	cancel()
	multicast.OnEvent(t.Context(), event.AgentUndeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestMulticastIsolatesInteractionBoundaryListeners(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewTextPart("original"))
	response, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := event.InteractionBoundary{
		Header: event.NewHeader("process-1"),
		Boundary: interaction.Event{
			Kind:     interaction.EventModelResponse,
			Round:    1,
			Final:    true,
			Response: response,
		},
	}

	var observed string
	multicast := event.NewMulticast()
	multicast.Add(event.ListenerFunc(func(_ context.Context, value event.Event) {
		value.(event.InteractionBoundary).Boundary.Response.Choices[0].Message.Parts[0].Text = "mutated"
	}))
	multicast.Add(event.ListenerFunc(func(_ context.Context, value event.Event) {
		observed = value.(event.InteractionBoundary).Boundary.Response.Text()
	}))
	multicast.OnEvent(t.Context(), boundary)

	if observed != "original" || boundary.Boundary.Response.Text() != "original" {
		t.Fatalf("observed=%q source=%q, want isolated original values", observed, boundary.Boundary.Response.Text())
	}
}

func TestNamedListener_ConcurrentDelivery(t *testing.T) {
	// Smoke test: NamedListener fn must tolerate concurrent OnEvent calls
	// from the multicast (the user closure is responsible for its own
	// synchronization). Verify no race when the callback is a goroutine-safe
	// counter.
	var (
		mu    sync.Mutex
		count int
	)
	listener := event.NewNamedListener("counter", func(context.Context, event.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	mc := event.NewMulticast()
	mc.Add(listener)

	const N = 100
	var wg sync.WaitGroup
	for range N {
		wg.Go(func() {
			mc.OnEvent(context.Background(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != N {
		t.Fatalf("count = %d, want %d", count, N)
	}
}
