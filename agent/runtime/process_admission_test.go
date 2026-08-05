package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

func TestFreshProcessOwnsFirstRunBeforeCreatedEvent(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		name := "Run"
		if asynchronous {
			name = "Start"
		}
		t.Run(name, func(t *testing.T) {
			engine := agent.MustNewEngine(runtime.Config{})
			definition := admissionAgent()
			mustDeploy(t, engine, definition)

			var attempted atomic.Bool
			admission := make(chan error, 1)
			listener := runtime.NewEventListener("created-reentry", func(ctx context.Context, published event.Event) {
				if _, ok := published.(event.ProcessCreated); !ok || !attempted.CompareAndSwap(false, true) {
					return
				}
				_, err := engine.ContinueAsync(ctx, published.ProcessID())
				admission <- err
			})
			options := core.ProcessOptions{Extensions: []core.Extension{listener}}

			if asynchronous {
				runHandle, err := engine.Start(t.Context(), definition, core.Input(word{Text: "lynx"}), options)
				if err != nil {
					t.Fatalf("Start: %v", err)
				}
				awaitRun(t, runHandle)
			} else {
				if _, err := engine.Run(t.Context(), definition, core.Input(word{Text: "lynx"}), options); err != nil {
					t.Fatalf("Run: %v", err)
				}
			}

			select {
			case err := <-admission:
				if !errors.Is(err, runtime.ErrProcessRunning) {
					t.Fatalf("Created listener ContinueAsync error = %v, want ErrProcessRunning", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Created listener did not attempt re-entry")
			}
		})
	}
}

func TestFreshChildOwnsFirstRunBeforeCreatedEvent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := agent.New(agent.Config{
		Name: "child-admission-parent",
		Actions: []agent.Action{agent.NewAction(
			"run-child",
			func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
				child, err := engine.RunChild(ctx, childDeployment, input)
				if err != nil {
					return parentOutput{}, err
				}
				output, ok := core.Result[subOutput](child)
				if !ok {
					return parentOutput{}, errors.New("child result is missing")
				}
				return parentOutput{Final: output.Doubled}, nil
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "child finished"})},
	})

	admission := make(chan error, 1)
	listener := runtime.NewSubtreeEventListener("child-created-reentry", func(ctx context.Context, published event.Event) {
		created, ok := published.(event.ProcessCreated)
		if !ok || created.ParentID == "" {
			return
		}
		_, err := engine.ContinueAsync(ctx, created.ProcessID())
		admission <- err
	})
	if _, err := engine.Run(
		t.Context(),
		parent,
		core.Input(subInput{Value: 21}),
		core.ProcessOptions{Extensions: []core.Extension{listener}},
	); err != nil {
		t.Fatalf("Run parent: %v", err)
	}
	select {
	case err := <-admission:
		if !errors.Is(err, runtime.ErrProcessRunning) {
			t.Fatalf("child Created listener ContinueAsync error = %v, want ErrProcessRunning", err)
		}
	case <-time.After(time.Second):
		t.Fatal("child Created listener did not attempt re-entry")
	}
}
