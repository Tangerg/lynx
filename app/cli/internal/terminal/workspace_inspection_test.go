package terminal

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type workspaceServiceStub struct {
	mu      sync.Mutex
	changes []workspace.Change
	calls   map[string]int
}

func newWorkspaceServiceStub() *workspaceServiceStub {
	return &workspaceServiceStub{
		changes: []workspace.Change{{Path: "main.go", Status: workspace.FileStatusModified}},
		calls:   make(map[string]int),
	}
}

func (stub *workspaceServiceStub) called(operation string) {
	stub.mu.Lock()
	stub.calls[operation]++
	stub.mu.Unlock()
}

func (stub *workspaceServiceStub) callCount(operation string) int {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.calls[operation]
}

func (stub *workspaceServiceStub) setChanges(changes ...workspace.Change) {
	stub.mu.Lock()
	stub.changes = append([]workspace.Change(nil), changes...)
	stub.mu.Unlock()
}

func (stub *workspaceServiceStub) Resolve(_ context.Context, request workspace.ResolveRequest) (workspace.Workspace, error) {
	stub.called("resolve")
	return workspace.Workspace{Path: request.Path, ProjectRoot: request.Path, Availability: workspace.Available}, nil
}

func (stub *workspaceServiceStub) List(context.Context) ([]workspace.Summary, error) {
	stub.called("list")
	return []workspace.Summary{{
		Workspace: workspace.Workspace{Path: "/tmp/lyra-cli-test", ProjectRoot: "/tmp/lyra-cli-test", Availability: workspace.Available},
		Name:      "lyra-cli-test", Sessions: 1,
	}}, nil
}

func (stub *workspaceServiceStub) Changes(context.Context, string) ([]workspace.Change, error) {
	stub.called("changes")
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return append([]workspace.Change(nil), stub.changes...), nil
}

func (stub *workspaceServiceStub) Diff(context.Context, workspace.DiffRequest) (workspace.Diff, error) {
	stub.called("diff")
	return workspace.Diff{Patch: "diff --git a/main.go b/main.go\n+var current = true"}, nil
}

func (stub *workspaceServiceStub) Head(context.Context, workspace.HeadRequest) (workspace.FileHead, error) {
	stub.called("head")
	return workspace.FileHead{Path: "main.go", Lines: []workspace.FileLine{{Number: 1, Text: "package main"}}}, nil
}

func (stub *workspaceServiceStub) Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error) {
	stub.called("search")
	return workspace.SearchResult{Matches: []workspace.Match{{Path: "main.go", Line: 1, Text: "package main"}}, Total: 1}, nil
}

func (stub *workspaceServiceStub) Files(context.Context, workspace.FilesRequest) (workspace.FileListing, error) {
	stub.called("files")
	return workspace.FileListing{Entries: []workspace.FileEntry{{Path: "main.go", Name: "main.go", Type: workspace.FileEntryFile}}}, nil
}

func (stub *workspaceServiceStub) Read(context.Context, workspace.ReadRequest) (workspace.FileContent, error) {
	stub.called("read")
	return workspace.FileContent{Path: "main.go", Content: "package main\n", Encoding: "utf-8", TotalLines: 1}, nil
}

type changeSourceStub struct {
	events chan changefeed.Event
}

type changeReaderFunc func(context.Context, string) ([]workspace.Change, error)

func (read changeReaderFunc) Changes(ctx context.Context, path string) ([]workspace.Change, error) {
	return read(ctx, path)
}

type changeSourceFunc func(context.Context, changefeed.Subscription) (changefeed.EventStream, error)

func (source changeSourceFunc) Supports(topic changefeed.Topic) bool {
	return topic == changefeed.FilesChanged
}

func (source changeSourceFunc) Subscribe(ctx context.Context, subscription changefeed.Subscription) (changefeed.EventStream, error) {
	return source(ctx, subscription)
}

func (stub *changeSourceStub) Supports(topic changefeed.Topic) bool {
	return topic == changefeed.FilesChanged
}

func (stub *changeSourceStub) Subscribe(ctx context.Context, _ changefeed.Subscription) (changefeed.EventStream, error) {
	return func(yield func(changefeed.Event, error) bool) {
		for {
			select {
			case event := <-stub.events:
				if !yield(event, nil) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

func runUIWithWorkspaceBackend(t *testing.T, service workspace.Service, source changefeed.Source) (*programtest.Host, func()) {
	t.Helper()
	backend := mock.New()
	backend.Instant = true
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspaces: service, Changes: source,
			Workspace: "/tmp/lyra-cli-test", Host: host,
		})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestWorkspaceCommandsConsumeEveryInspectionRead(t *testing.T) {
	service := newWorkspaceServiceStub()
	host, stop := runUIWithWorkspaceBackend(t, service, nil)
	host.Shows(t, "Ask lyra")

	commands := []struct {
		input string
		show  string
		call  string
	}{
		{input: "/workspaces", show: "Runtime workspaces", call: "list"},
		{input: "/changes", show: "Workspace changes", call: "changes"},
		{input: "/diff main.go", show: "Workspace diff", call: "diff"},
		{input: "/preview main.go", show: "File preview", call: "head"},
		{input: "/grep package", show: "Workspace search", call: "search"},
		{input: "/browse", show: "Workspace files", call: "files"},
		{input: "/read main.go", show: "Workspace file", call: "read"},
	}
	for _, command := range commands {
		host.Type(command.input)
		host.Press(input.Enter)
		host.Shows(t, command.show)
		if service.callCount(command.call) == 0 {
			t.Fatalf("%s did not call %s", command.input, command.call)
		}
		host.Press(input.Esc)
		host.Shows(t, "Ask lyra")
	}
	stop()
}

func TestFileInvalidationsRefetchAuthoritativeChangesAndRecoverSequenceGaps(t *testing.T) {
	service := newWorkspaceServiceStub()
	source := &changeSourceStub{events: make(chan changefeed.Event, 4)}
	host, stop := runUIWithWorkspaceBackend(t, service, source)
	host.Shows(t, "Δ1")
	host.Type("/changes")
	host.Press(input.Enter)
	host.Shows(t, "Workspace changes")

	service.setChanges(
		workspace.Change{Path: "main.go", Status: workspace.FileStatusModified},
		workspace.Change{Path: "new.go", Status: workspace.FileStatusUntracked},
	)
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 1,
		WatchID: workspaceWatchID, Workspace: "/tmp/lyra-cli-test", Paths: []string{"new.go"},
	}
	host.Shows(t, "2 files")
	host.Shows(t, "new.go")
	host.Press(input.Esc)
	host.Shows(t, "Δ2")

	service.setChanges(
		workspace.Change{Path: "main.go", Status: workspace.FileStatusModified},
		workspace.Change{Path: "new.go", Status: workspace.FileStatusUntracked},
		workspace.Change{Path: "old.go", Status: workspace.FileStatusDeleted},
	)
	// Sequence 2 was deliberately missed. The monitor must detect the gap and
	// refetch instead of trusting the event payload as state.
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.FilesChanged), Sequence: 3,
		WatchID: workspaceWatchID, Workspace: "/tmp/lyra-cli-test", Paths: []string{"old.go"},
	}
	host.Shows(t, "Δ3")
	stop()
}

func TestWorkspaceMonitorSubscribesBeforeItsAuthoritativeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var mu sync.Mutex
	var order []string
	record := func(step string) {
		mu.Lock()
		order = append(order, step)
		mu.Unlock()
	}
	monitor := runtimeChangeMonitor{
		workspace: "/workspace",
		source: changeSourceFunc(func(ctx context.Context, _ changefeed.Subscription) (changefeed.EventStream, error) {
			record("subscribe")
			return func(func(changefeed.Event, error) bool) { <-ctx.Done() }, nil
		}),
		repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			record("read")
			return []workspace.Change{}, nil
		}),
		applyFiles: func([]workspace.Change) error {
			record("apply")
			cancel()
			return nil
		},
	}
	monitor.run(ctx)
	mu.Lock()
	defer mu.Unlock()
	want := []string{"subscribe", "read", "apply"}
	if !slices.Equal(order, want) {
		t.Fatalf("monitor order = %v, want %v", order, want)
	}
}

func TestWorkspaceMonitorReadsFilesWhenTheChangeSourceCannotWatchThem(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event), subscription: make(chan changefeed.Subscription, 1),
		supported: []changefeed.Topic{changefeed.SessionsChanged},
	}
	read := make(chan struct{}, 1)
	applied := make(chan []workspace.Change, 1)
	monitor := runtimeChangeMonitor{
		workspace: "/workspace", source: source,
		repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			read <- struct{}{}
			return []workspace.Change{{Path: "main.go", Status: workspace.FileStatusModified}}, nil
		}),
		applyFiles: func(changes []workspace.Change) error {
			applied <- changes
			cancel()
			return nil
		},
	}
	done := make(chan struct{})
	go func() {
		monitor.run(ctx)
		close(done)
	}()

	subscription := <-source.subscription
	if !slices.Equal(subscription.Topics, []changefeed.Topic{changefeed.SessionsChanged}) || len(subscription.Watches) != 0 {
		t.Fatalf("subscription = %+v", subscription)
	}
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("file projection was not read")
	}
	select {
	case changes := <-applied:
		if len(changes) != 1 || changes[0].Path != "main.go" {
			t.Fatalf("applied changes = %+v", changes)
		}
	case <-time.After(time.Second):
		t.Fatal("file projection was not applied")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor did not stop")
	}
}

var _ workspace.Service = (*workspaceServiceStub)(nil)
var _ changefeed.Source = (*changeSourceStub)(nil)
