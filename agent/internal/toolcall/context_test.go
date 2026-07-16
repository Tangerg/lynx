package toolcall

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

func TestBoundCallDoesNotCrossProcessBoundary(t *testing.T) {
	call := chat.ToolCall{ID: "call-1", Name: "delegate", Arguments: `{}`}

	unscoped := Bind(t.Context(), call)
	if got, ok := FromContext(unscoped); !ok || got != call {
		t.Fatalf("unscoped call = (%#v, %v), want (%#v, true)", got, ok, call)
	}
	if _, ok := FromContext(core.WithProcessView(unscoped, processView{id: "child"})); ok {
		t.Fatal("unscoped call leaked into a process")
	}

	parent := processView{id: "parent"}
	scoped := Bind(core.WithProcessView(t.Context(), parent), call)
	if got, ok := FromContext(scoped); !ok || got != call {
		t.Fatalf("parent-scoped call = (%#v, %v), want (%#v, true)", got, ok, call)
	}
	if _, ok := FromContext(core.WithProcessView(scoped, processView{id: "child"})); ok {
		t.Fatal("parent call leaked into child process")
	}
	if _, ok := FromContext(context.WithoutCancel(scoped)); !ok {
		t.Fatal("same-process derived context lost bound call")
	}
}

type processView struct {
	id string
}

func (p processView) ID() string                         { return p.id }
func (processView) ParentID() string                     { return "" }
func (processView) Deployment() core.DeploymentRef       { return core.DeploymentRef{} }
func (processView) StartedAt() time.Time                 { return time.Time{} }
func (processView) Status() core.ProcessStatus           { return core.StatusRunning }
func (processView) Goal() *core.Goal                     { return nil }
func (processView) Blackboard() core.BlackboardReader    { return nil }
func (processView) Failure() error                       { return nil }
func (processView) Suspension() *interaction.Suspension  { return nil }
func (processView) WorldState() core.WorldState          { return nil }
func (processView) Usage() (float64, int, int)           { return 0, 0, 0 }
func (processView) ModelCalls() []core.ModelCall         { return nil }
func (processView) EmbeddingCalls() []core.EmbeddingCall { return nil }
