package event_test

import (
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/event"
)

func TestNamedListener_NameAndOnEvent(t *testing.T) {
	var got []string
	listener := event.NewNamedListener("collector", func(e event.Event) {
		got = append(got, e.EventName())
	})

	if listener.Name() != "collector" {
		t.Fatalf("Name() = %q, want %q", listener.Name(), "collector")
	}

	mc := event.NewMulticast()
	mc.Add(listener)
	mc.OnEvent(event.AgentDeployedEvent{BaseEvent: event.NewBaseEvent(""), AgentName: "x"})
	mc.OnEvent(event.AgentUndeployedEvent{BaseEvent: event.NewBaseEvent(""), AgentName: "x"})

	if len(got) != 2 {
		t.Fatalf("captured %d events, want 2: %v", len(got), got)
	}
	if got[0] != "agent_deployed" || got[1] != "agent_undeployed" {
		t.Fatalf("captured order = %v", got)
	}
}

func TestNamedListener_NilFnIsNoop(t *testing.T) {
	listener := event.NewNamedListener("noop", nil)

	// Should not panic.
	listener.OnEvent(event.AgentDeployedEvent{BaseEvent: event.NewBaseEvent(""), AgentName: "x"})
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
	listener := event.NewNamedListener("counter", func(_ event.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	mc := event.NewMulticast()
	mc.Add(listener)

	const N = 100
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mc.OnEvent(event.AgentDeployedEvent{BaseEvent: event.NewBaseEvent(""), AgentName: "x"})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != N {
		t.Fatalf("count = %d, want %d", count, N)
	}
}
