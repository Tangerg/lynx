package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type srWord struct{ Text string }
type srWordCount struct{ Count int }

func buildSessionAgent() *core.Agent {
	return agent.New("session-agent").
		Actions(agent.NewAction("count",
			func(ctx context.Context, pc *core.ProcessContext, in srWord) (srWordCount, error) {
				// Echo whether the session is wired through ProcessOptions.
				if pc.Options != nil && pc.Options.Session != nil {
					pc.Blackboard.Set("session_id_seen_in_action", pc.Options.Session.ID)
				}
				return srWordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[srWordCount](core.Goal{Description: "counted in session"})).
		Build()
}

func TestPlatform_RunInSession_NilSession(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	_, err := platform.RunInSession(context.Background(), a, nil, nil, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for nil session")
	}
}

func TestPlatform_RunInSession_EmptyID(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	sess := &core.Session{} // ID empty
	_, err := platform.RunInSession(context.Background(), a, sess, nil, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestPlatform_RunInSession_NoStore_PropagatesSession(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	sess := core.NewSession("conv-123", "alice", "session-agent")
	proc, err := platform.RunInSession(
		context.Background(), a, &sess,
		map[string]any{core.DefaultBindingName: srWord{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run in session: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	// The action wrote the session id it observed onto the blackboard.
	seen, ok := proc.Blackboard().Get("session_id_seen_in_action")
	if !ok || seen != "conv-123" {
		t.Errorf("session not propagated; got blackboard value %v", seen)
	}
	// Without a SessionStore configured the runtime should not touch
	// the session (no save would happen anyway).
	if !sess.UpdatedAt.Equal(sess.StartedAt) {
		t.Errorf("UpdatedAt should equal StartedAt when no SessionStore configured; got %v vs %v",
			sess.UpdatedAt, sess.StartedAt)
	}
}

func TestPlatform_RunInSession_WithStore_PersistsSession(t *testing.T) {
	store := core.NewInMemorySessionStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{
		SessionStore: store,
	})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	sess := core.NewSession("conv-A", "bob", "session-agent")
	_, err := platform.RunInSession(
		context.Background(), a, &sess,
		map[string]any{core.DefaultBindingName: srWord{Text: "x"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(context.Background(), "conv-A")
	if err != nil {
		t.Fatalf("store load: %v", err)
	}
	if loaded.AgentName != "session-agent" || loaded.UserID != "bob" {
		t.Errorf("loaded session: %#v", loaded)
	}
}

func TestPlatform_RunInSession_DerivesAgentName(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	sess := core.Session{ID: "c-1"} // no AgentName set
	_, err := platform.RunInSession(
		context.Background(), a, &sess,
		map[string]any{core.DefaultBindingName: srWord{Text: "hi"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if sess.AgentName != "session-agent" {
		t.Errorf("AgentName: want auto-derived 'session-agent', got %q", sess.AgentName)
	}
}

// buildLineageParentAgent spawns childAgent as a protected-only child and
// records the child's session id + ParentID onto its own blackboard so a
// test can assert the lineage the runtime stamps at spawn time.
func buildLineageParentAgent(platform *runtime.Platform, childAgent *core.Agent) *core.Agent {
	return agent.New("lineage-parent").
		Actions(agent.NewAction("delegate",
			func(ctx context.Context, pc *core.ProcessContext, in srWord) (srWordCount, error) {
				child, err := runtime.SpawnChildProtectedOnly(ctx, platform, childAgent, in)
				if err != nil {
					return srWordCount{}, err
				}
				if s := child.Options().Session; s != nil {
					pc.Blackboard.Set("child_session_id", s.ID)
					pc.Blackboard.Set("child_parent_id", s.ParentID)
				}
				return srWordCount{Count: 1}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[srWordCount](core.Goal{Description: "delegated"})).
		Build()
}

// TestSpawnChild_LinksChildSessionToParent pins the session-lineage
// contract: a spawned child runs under its OWN session (independent
// conversation id, so its chat-memory history is isolated) whose ParentID
// records the parent's conversation id.
func TestSpawnChild_LinksChildSessionToParent(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	child := buildSessionAgent()
	parent := buildLineageParentAgent(platform, child)
	mustDeploy(t, platform, child, parent)

	sess := core.NewSession("parent-conv", "alice", "lineage-parent")
	proc, err := platform.RunInSession(
		context.Background(), parent, &sess,
		map[string]any{core.DefaultBindingName: srWord{Text: "hi"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run in session: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	childID, _ := proc.Blackboard().Get("child_session_id")
	parentID, _ := proc.Blackboard().Get("child_parent_id")
	if childID == "" || childID == "parent-conv" {
		t.Errorf("child session id should be independent of the parent; got %v", childID)
	}
	if parentID != "parent-conv" {
		t.Errorf("child session ParentID = %v, want %q (the parent conversation)", parentID, "parent-conv")
	}
}
