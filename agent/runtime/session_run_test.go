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
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	_, err := platform.RunInSession(context.Background(), a, nil, nil, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for nil session")
	}
}

func TestPlatform_RunInSession_EmptyID(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	a := buildSessionAgent()
	mustDeploy(t, platform, a)

	sess := &core.Session{} // ID empty
	_, err := platform.RunInSession(context.Background(), a, sess, nil, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestPlatform_RunInSession_NoStore_PropagatesSession(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
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
	platform := agent.NewPlatform(&runtime.PlatformConfig{
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
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
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
