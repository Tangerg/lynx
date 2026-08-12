package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	workspaceadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/knowledgefile"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestWorkspaceSubscribe_GitWatch verifies that a real staged index transition
// surfaces a debounced resync without recursively watching the working tree.
func TestWorkspaceSubscribe_GitWatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	fileWatchGitCommand(t, dir, "init", "-q")
	tracked := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("v0\n"), 0o644); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	s := newWorkspaceServer(dir)
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)

	// A git operation semantically changes the staged index → expect a debounced resync.
	if err := os.WriteFile(tracked, []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	fileWatchGitCommand(t, dir, "add", "tracked.txt")
	select {
	case ev := <-events:
		if ev.Type != "resync" {
			t.Fatalf("event = %+v, want resync", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no resync within 3s of a staged index change")
	}
}

// TestWorkspaceSubscribe_NonRepoInert: a watch on a cwd that isn't a git repo
// contributes no watcher (and doesn't error) — the broadcast stream still works.
func TestWorkspaceSubscribe_NonRepoInert(t *testing.T) {
	dir := t.TempDir() // no .git
	s := newWorkspaceServer(dir)
	s.workspaceHub = newWorkspaceHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicSkillsChanged},
		Watches: []protocol.WatchSpec{{WatchID: "w1", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe (non-repo) must not error: %v", err)
	}
	events := drainSeq(ctx, seq)
	// Broadcast events still flow on the subscription.
	s.workspaceHub.publish(protocol.RuntimeEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast event not delivered on a non-repo subscription")
	}
}

// TestWorkspaceSubscribe_ExternalAuthoredFiles verifies that the two
// file-backed, user-authored resources converge when another process edits
// them. Git observation is deliberately irrelevant here: neither LYRA.md nor
// hooks.json needs to be staged before its query projection becomes stale.
func TestWorkspaceSubscribe_ExternalAuthoredFiles(t *testing.T) {
	dir := t.TempDir()
	s := newWorkspaceServer(dir)
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{
			protocol.TopicFilesChanged,
			protocol.TopicKnowledgeChanged,
			protocol.TopicHooksChanged,
		},
		Watches: []protocol.WatchSpec{{WatchID: "authored", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)

	if err := os.WriteFile(filepath.Join(dir, "LYRA.md"), []byte("external knowledge\n"), 0o644); err != nil {
		t.Fatalf("write knowledge: %v", err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeKnowledgeChanged)

	if err := os.MkdirAll(filepath.Join(dir, ".lyra"), 0o755); err != nil {
		t.Fatalf("create hook directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lyra", "hooks.json"), []byte(`{"hooks":[]}`), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeHooksChanged)
}

func TestWorkspaceSubscribe_GlobalAuthoredFilesDoNotRequireWorkspaceWatch(t *testing.T) {
	workspaceRoot := t.TempDir()
	knowledgeHome := t.TempDir()
	hooksHome := t.TempDir()
	authored, err := workspaceadapter.NewAuthoredWatcher(knowledgeHome, hooksHome)
	if err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceServerWithConfig(workspaceRoot, workspaceTestConfig{AuthoredWatcher: authored})
	s.workspaceHub = newWorkspaceHub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{
		protocol.TopicKnowledgeChanged, protocol.TopicHooksChanged,
	}})
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, seq)
	if err := os.WriteFile(filepath.Join(knowledgeHome, "LYRA.md"), []byte("global knowledge"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeKnowledgeChanged)
	if err := os.MkdirAll(filepath.Join(hooksHome, ".lyra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksHome, ".lyra", "hooks.json"), []byte(`{"hooks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeHooksChanged)

	if err := os.WriteFile(filepath.Join(workspaceRoot, "LYRA.md"), []byte("unwatched workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		t.Fatalf("workspace path without a watch produced %+v", event)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWorkspaceSubscribe_KnowledgeUpdateDoesNotDoublePublishFromFileObservation(t *testing.T) {
	dir := t.TempDir()
	store, err := knowledgefile.New(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	surfaces := newWorkspaceSurfaces(dir, workspaceTestConfig{Knowledge: store})
	s := &Server{}
	applyWorkspaceSurfaces(s, surfaces)
	s.workspaceHub = newWorkspaceHub()
	s.workspaceKnowledge = workspaceapp.NewKnowledge(
		surfaces.roots, workspacepath.Resolver{}, store, surfaces.authoredWatch,
		func(notice invalidation.Notice) {
			if event, ok := runtimeEventFor(notice); ok {
				s.workspaceHub.publish(event)
			}
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicKnowledgeChanged},
		Watches: []protocol.WatchSpec{{WatchID: "knowledge", Workspace: protocol.WorkspaceRef{Path: dir}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drainSeq(ctx, seq)
	current, err := s.GetKnowledge(ctx, protocol.GetKnowledgeRequest{
		Scope: protocol.KnowledgeScopeCWD, Workspace: &protocol.WorkspaceRef{Path: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateKnowledge(ctx, protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScopeCWD, Workspace: &protocol.WorkspaceRef{Path: dir},
		ExpectedRevision: current.Revision, Content: "one event\n",
	}); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEventType(t, events, protocol.RuntimeKnowledgeChanged)
	select {
	case event := <-events:
		t.Fatalf("knowledge.update published a duplicate observation event: %+v", event)
	case <-time.After(500 * time.Millisecond):
	}
}

func assertRuntimeEventType(t *testing.T, events <-chan protocol.RuntimeEvent, want protocol.RuntimeEventType) {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != want {
			t.Fatalf("event = %+v, want %s", event, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s event within 3s of an external file change", want)
	}
}

func fileWatchGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

// TestRunEffectsNudgePublishesFileChange verifies the application nudge adapter
// reaches the workspace event hub. Tool-item-to-nudge decisions belong to and
// are tested in application/runs.
func TestRunEffectsNudgePublishesFileChange(t *testing.T) {
	s := &Server{workspaceHub: newWorkspaceHub()}
	events, unsub := s.workspaceHub.subscribe()
	defer unsub()

	// Wire the production seam: the run effects publish nudges through the
	// notifier, and the hub observes it (mapping to the wire files.changed).
	fc := &testNotification[workspaceapp.FileChangeNotice]{}
	s.observeFileChanges(fc)
	effects := runsegment.New(runsegment.Config{PublishFileChanges: fc.Publish})

	effects.Nudge("/proj", []string{"src/a.go"})
	select {
	case ev := <-events:
		if ev.Type != "files.changed" || ev.Workspace == nil || ev.Workspace.Path != "/proj" || len(ev.Paths) != 1 || ev.Paths[0] != "src/a.go" {
			t.Fatalf("event = %+v, want files.changed cwd=/proj [src/a.go]", ev)
		}
	default:
		t.Fatal("write tool call must publish files.changed")
	}

}

// TestWorkspaceSubscribe_MissingWatchID rejects a watch with no id.
func TestWorkspaceSubscribe_MissingWatchID(t *testing.T) {
	s := newWorkspaceServer(t.TempDir())
	s.workspaceHub = newWorkspaceHub()
	if _, _, err := s.SubscribeRuntime(context.Background(), protocol.RuntimeSubscribeRequest{
		Watches: []protocol.WatchSpec{{}},
	}); err == nil {
		t.Fatal("watch missing watchId must be invalid_params")
	}
}
