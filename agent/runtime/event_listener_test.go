package runtime_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

var eventListenerDeployment = core.DeploymentRef{Name: "x", Digest: "digest"}

func TestNamedEventListener(t *testing.T) {
	var got []event.Kind
	listener := runtime.NewEventListener("collector", func(_ context.Context, published event.Event) {
		got = append(got, published.Kind())
	})

	if listener.Name() != "collector" {
		t.Fatalf("Name() = %q, want collector", listener.Name())
	}

	multicast := event.NewMulticast()
	multicast.Subscribe(listener)
	multicast.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: eventListenerDeployment})
	multicast.OnEvent(t.Context(), event.AgentUndeployed{Header: event.NewHeader(""), Deployment: eventListenerDeployment})

	if len(got) != 2 || got[0] != "agent_deployed" || got[1] != "agent_undeployed" {
		t.Fatalf("captured order = %v, want [agent_deployed agent_undeployed]", got)
	}
}

func TestNamedEventListenerNilCallback(t *testing.T) {
	listener := runtime.NewEventListener("nop", nil)
	listener.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: eventListenerDeployment})
}

func TestNamedEventListenerConcurrentDelivery(t *testing.T) {
	var (
		mu    sync.Mutex
		count int
	)
	listener := runtime.NewEventListener("counter", func(context.Context, event.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	multicast := event.NewMulticast()
	multicast.Subscribe(listener)

	const deliveries = 100
	var group sync.WaitGroup
	for range deliveries {
		group.Go(func() {
			multicast.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: eventListenerDeployment})
		})
	}
	group.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != deliveries {
		t.Fatalf("count = %d, want %d", count, deliveries)
	}
}

func TestNamedSubtreeEventListenerImplementsMarker(t *testing.T) {
	listener := runtime.NewSubtreeEventListener("subtree", nil)
	var capability runtime.SubtreeEventListener = listener
	capability.ObserveSubtree()
}
