package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

var listenerDeployment = core.DeploymentRef{Name: "x", Digest: "digest"}

type pointerListener struct{ calls int }

func (l *pointerListener) OnEvent(context.Context, event.Event) { l.calls++ }

type pointerEvent struct{}

func (*pointerEvent) Timestamp() time.Time { return time.Time{} }
func (*pointerEvent) ProcessID() string    { return "" }
func (*pointerEvent) Kind() event.Kind     { return "test" }

func TestMulticastUnsubscribeListenerFunc(t *testing.T) {
	var calls int
	multicast := event.NewMulticast()
	unsubscribe := multicast.Subscribe(event.ListenerFunc(func(context.Context, event.Event) {
		calls++
	}))

	multicast.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
	unsubscribe()
	unsubscribe()
	multicast.OnEvent(t.Context(), event.AgentUndeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestMulticastIgnoresTypedNilListener(t *testing.T) {
	var listener *pointerListener
	multicast := event.NewMulticast()
	unsubscribe := multicast.Subscribe(listener)
	multicast.OnEvent(t.Context(), event.AgentDeployed{Header: event.NewHeader(""), Deployment: listenerDeployment})
	unsubscribe()
}

func TestMulticastIgnoresTypedNilEvent(t *testing.T) {
	listener := &pointerListener{}
	multicast := event.NewMulticast()
	multicast.Subscribe(listener)
	multicast.OnEvent(t.Context(), (*pointerEvent)(nil))
	if listener.calls != 0 {
		t.Fatalf("listener calls = %d, want none", listener.calls)
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
	multicast.Subscribe(event.ListenerFunc(func(_ context.Context, value event.Event) {
		value.(event.InteractionBoundary).Boundary.Response.Choices[0].Message.Parts[0].Text = "mutated"
	}))
	multicast.Subscribe(event.ListenerFunc(func(_ context.Context, value event.Event) {
		observed = value.(event.InteractionBoundary).Boundary.Response.Text()
	}))
	multicast.OnEvent(t.Context(), boundary)

	if observed != "original" || boundary.Boundary.Response.Text() != "original" {
		t.Fatalf("observed=%q source=%q, want isolated original values", observed, boundary.Boundary.Response.Text())
	}
}

// TestActionEventsExposeOnlyImmutableDescriptions proves an observer can mutate
// accessor snapshots without changing the event seen by another observer.
func TestActionEventsExposeOnlyImmutableDescriptions(t *testing.T) {
	metadata := core.ActionMetadata{
		Name:          "publish",
		Preconditions: core.ConditionSet{"ready": core.True},
		ToolRoles:     []string{"coding"},
	}
	started := event.ActionStarted{
		Header: event.NewHeader("process-1"),
		Action: metadata.Descriptor(),
	}

	var observedCondition core.Truth
	var observedGroup string
	multicast := event.NewMulticast()
	multicast.Subscribe(event.ListenerFunc(func(_ context.Context, value event.Event) {
		action := value.(event.ActionStarted).Action
		preconditions := action.Preconditions()
		preconditions["ready"] = core.False
		groups := action.ToolRoles()
		groups[0] = "mutated"
	}))
	multicast.Subscribe(event.ListenerFunc(func(_ context.Context, value event.Event) {
		action := value.(event.ActionStarted).Action
		observedCondition = action.Preconditions()["ready"]
		observedGroup = action.ToolRoles()[0]
	}))
	multicast.OnEvent(t.Context(), started)

	if observedCondition != core.True || observedGroup != "coding" {
		t.Fatalf("observed %v/%q, want the values the event was published with", observedCondition, observedGroup)
	}
	if started.Action.Preconditions()["ready"] != core.True || started.Action.ToolRoles()[0] != "coding" {
		t.Fatalf("publishing mutated the source descriptor")
	}
}
