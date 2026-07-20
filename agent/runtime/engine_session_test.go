package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type srWord struct{ Text string }
type srWordCount struct{ Count int }

func buildSessionAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "session-agent", Actions: []agent.Action{agent.NewAction("count", func(ctx context.Context, pc *core.ProcessContext, in srWord) (srWordCount, error) {
		if session, ok := pc.Session(); ok {
			pc.Blackboard().Store("session_id_seen_in_action", session.ID)
			pc.Blackboard().Store("session_parent_id_seen_in_action", session.ParentID)
		}
		return srWordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[srWordCount](core.GoalConfig{Description: "counted in session"})}})
}

func TestEngine_RunInSession_NilSession(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)

	_, err := engine.RunInSession(context.Background(), a, nil, core.Bindings{}, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for nil session")
	}
}

func TestEngine_RunInSession_EmptyID(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)

	sess := &core.Session{} // ID empty
	_, err := engine.RunInSession(context.Background(), a, sess, core.Bindings{}, core.ProcessOptions{})
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestEngine_RunInSession_NoStore_PropagatesSession(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)

	sess := core.NewSession("conv-123", "alice", "session-agent")
	previousUpdate := time.Unix(1, 0).UTC()
	sess.StartedAt = previousUpdate
	sess.UpdatedAt = previousUpdate
	proc, err := engine.RunInSession(
		context.Background(), a, &sess,
		core.Input(srWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run in session: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	// The action wrote the session id it observed onto the blackboard.
	seen, ok := proc.Blackboard().Load("session_id_seen_in_action")
	if !ok || seen != "conv-123" {
		t.Errorf("session not propagated; got blackboard value %v", seen)
	}
	if !sess.UpdatedAt.After(previousUpdate) {
		t.Errorf("UpdatedAt should advance without a SessionStore; got %v after %v",
			sess.UpdatedAt, previousUpdate)
	}
}

func TestEngine_RunInSession_WithStore_PersistsSession(t *testing.T) {
	store := core.NewMemorySessionStore()
	engine := agent.MustNewEngine(runtime.Config{
		SessionStore: store,
	})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)

	sess := core.NewSession("conv-A", "bob", "session-agent")
	_, err := engine.RunInSession(
		context.Background(), a, &sess,
		core.Input(srWord{Text: "x"}),
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

func TestEngine_RunInSession_DerivesAgentName(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)

	sess := core.NewSession("c-1", "", "")
	_, err := engine.RunInSession(
		context.Background(), a, &sess,
		core.Input(srWord{Text: "hi"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if sess.AgentName != "session-agent" {
		t.Errorf("AgentName: want auto-derived 'session-agent', got %q", sess.AgentName)
	}
}

func TestEngine_RunInSession_UsesSessionAgentWhenDefinitionIsNil(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)
	session := core.NewSession("c-1", "user-1", a.Name())

	process, err := engine.RunInSession(
		t.Context(), nil, &session,
		core.Input(srWord{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunInSession: %v", err)
	}
	if process.Deployment().Name != a.Name() {
		t.Fatalf("deployment = %s, want agent %q", process.Deployment(), a.Name())
	}
}

func TestEngine_RunInSession_RejectsAgentIdentityConflict(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	a := buildSessionAgent()
	session := core.NewSession("c-1", "user-1", "other-agent")

	_, err := engine.RunInSession(t.Context(), a, &session, core.Bindings{}, core.ProcessOptions{})
	if !errors.Is(err, core.ErrInvalidSession) {
		t.Fatalf("RunInSession error = %v, want ErrInvalidSession", err)
	}
	if _, deployed := engine.ActiveDeployment(a.Name()); deployed {
		t.Fatal("identity conflict deployed the rejected agent")
	}
}

type cancellationAwareSessionStore struct {
	saveContexts []error
	failAt       int
	failure      error
}

func (s *cancellationAwareSessionStore) Save(ctx context.Context, _ core.Session) error {
	s.saveContexts = append(s.saveContexts, ctx.Err())
	if len(s.saveContexts) == s.failAt {
		return s.failure
	}
	return nil
}

func (*cancellationAwareSessionStore) Load(context.Context, string) (core.Session, error) {
	return core.Session{}, core.ErrSessionNotFound
}

func cancellationSessionAgent(cancel context.CancelFunc) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name: "cancel-session",
		Actions: []agent.Action{agent.NewAction("cancel", func(context.Context, *core.ProcessContext, srWord) (srWordCount, error) {
			cancel()
			return srWordCount{}, context.Canceled
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[srWordCount](core.GoalConfig{Description: "cancel"})},
	})
}

func TestEngine_RunInSession_FinalSaveSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	store := new(cancellationAwareSessionStore)
	engine := agent.MustNewEngine(runtime.Config{SessionStore: store})
	a := cancellationSessionAgent(cancel)
	mustDeploy(t, engine, a)
	session := core.NewSession("c-1", "user-1", a.Name())

	_, _ = engine.RunInSession(ctx, a, &session, core.Input(
		srWord{Text: "lynx"}),

		core.ProcessOptions{})
	if len(store.saveContexts) != 2 {
		t.Fatalf("Save calls = %d, want pre- and post-dispatch", len(store.saveContexts))
	}
	if store.saveContexts[0] != nil || store.saveContexts[1] != nil {
		t.Fatalf("Save context errors = %v, want both live", store.saveContexts)
	}
}

func TestEngine_RunInSession_PreservesRunAndFinalSaveErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	finalSaveErr := errors.New("final session save failed")
	store := &cancellationAwareSessionStore{failAt: 2, failure: finalSaveErr}
	engine := agent.MustNewEngine(runtime.Config{SessionStore: store})
	a := buildSessionAgent()
	mustDeploy(t, engine, a)
	session := core.NewSession("c-1", "user-1", a.Name())

	_, err := engine.RunInSession(ctx, a, &session, core.Input(
		srWord{Text: "lynx"}),

		core.ProcessOptions{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, finalSaveErr) {
		t.Fatalf("RunInSession error = %v, want cancellation and final save failure", err)
	}
}

// buildLineageParentAgent spawns childAgent as a protected-only child and
// records the child's session id + ParentID onto its own blackboard so a
// test can assert the lineage the runtime stamps at spawn time.
func buildLineageParentAgent(engine *runtime.Engine, childDeployment *runtime.Deployment) *core.Agent {
	return agent.New(agent.AgentConfig{Name: "lineage-parent", Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, pc *core.ProcessContext, in srWord) (srWordCount, error) {
		child, err := engine.RunChild(ctx, childDeployment, in)
		if err != nil {
			return srWordCount{}, err
		}
		childSessionID, _ := child.Blackboard().Load("session_id_seen_in_action")
		childParentID, _ := child.Blackboard().Load("session_parent_id_seen_in_action")
		pc.Blackboard().Store("child_session_id", childSessionID)
		pc.Blackboard().Store("child_parent_id", childParentID)
		return srWordCount{Count: 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[srWordCount](core.GoalConfig{Description: "delegated"})}})
}

// TestRunChildLinksSessionToParent pins the session-lineage
// contract: a spawned child runs under its OWN session (independent
// conversation id, so its chat history history is isolated) whose ParentID
// records the parent's conversation id.
func TestRunChildLinksSessionToParent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	child := buildSessionAgent()
	childDeployment, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := buildLineageParentAgent(engine, childDeployment)
	mustDeploy(t, engine, parent)

	sess := core.NewSession("parent-conv", "alice", "lineage-parent")
	proc, err := engine.RunInSession(
		context.Background(), parent, &sess,
		core.Input(srWord{Text: "hi"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run in session: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	childID, _ := proc.Blackboard().Load("child_session_id")
	parentID, _ := proc.Blackboard().Load("child_parent_id")
	if childID == "" || childID == "parent-conv" {
		t.Errorf("child session id should be independent of the parent; got %v", childID)
	}
	if parentID != "parent-conv" {
		t.Errorf("child session ParentID = %v, want %q (the parent conversation)", parentID, "parent-conv")
	}
}

// TestRunChildPersistsSession verifies that when the engine
// has a ChildSessionStore, a spawned child's session is saved there (with its
// ParentID), so the delegation lineage is durably queryable.
func TestRunChildPersistsSession(t *testing.T) {
	store := core.NewMemorySessionStore()
	engine := agent.MustNewEngine(runtime.Config{ChildSessionStore: store})
	child := buildSessionAgent()
	childDeployment, err := engine.Deploy(child)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parent := buildLineageParentAgent(engine, childDeployment)
	mustDeploy(t, engine, parent)

	sess := core.NewSession("root-conv", "alice", "lineage-parent")
	proc, err := engine.RunInSession(
		context.Background(), parent, &sess,
		core.Input(srWord{Text: "hi"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run in session: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	childID, _ := proc.Blackboard().Load("child_session_id")
	id, _ := childID.(string)
	if id == "" {
		t.Fatal("child session id not recorded")
	}
	saved, err := store.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("child session not persisted to store: %v", err)
	}
	if saved.ParentID != "root-conv" {
		t.Errorf("persisted child ParentID = %q, want %q", saved.ParentID, "root-conv")
	}
}
