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
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type workspaceServiceStub struct {
	mu          sync.Mutex
	changes     []workspace.Change
	calls       map[string]int
	diffRequest workspace.DiffRequest
	headRequest workspace.HeadRequest
	search      workspace.SearchRequest
	files       workspace.FilesRequest
	read        workspace.ReadRequest
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
	lastActive := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	return []workspace.Summary{{
		Workspace: workspace.Workspace{Path: "/tmp/lyra-cli-test", ProjectRoot: "/tmp/project-root", Availability: workspace.Available},
		Name:      "lyra-cli-test", Sessions: 1, LastActive: &lastActive,
	}}, nil
}

func (stub *workspaceServiceStub) Changes(context.Context, string) ([]workspace.Change, error) {
	stub.called("changes")
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return append([]workspace.Change(nil), stub.changes...), nil
}

func (stub *workspaceServiceStub) Diff(_ context.Context, request workspace.DiffRequest) (workspace.Diff, error) {
	stub.called("diff")
	stub.mu.Lock()
	stub.diffRequest = request
	stub.mu.Unlock()
	return workspace.Diff{Patch: "diff --git a/main.go b/main.go\n+var current = true"}, nil
}

func (stub *workspaceServiceStub) lastDiffRequest() workspace.DiffRequest {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.diffRequest
}

func (stub *workspaceServiceStub) Head(_ context.Context, request workspace.HeadRequest) (workspace.FileHead, error) {
	stub.called("head")
	stub.mu.Lock()
	stub.headRequest = request
	stub.mu.Unlock()
	return workspace.FileHead{Path: "main.go", Lines: []workspace.FileLine{{Number: 1, Text: "package main"}}}, nil
}

func (stub *workspaceServiceStub) Search(_ context.Context, request workspace.SearchRequest) (workspace.SearchResult, error) {
	stub.called("search")
	stub.mu.Lock()
	stub.search = request
	stub.mu.Unlock()
	return workspace.SearchResult{Matches: []workspace.Match{{Path: "main.go", Line: 1, Text: "package main"}}, Total: 1}, nil
}

func (stub *workspaceServiceStub) Files(_ context.Context, request workspace.FilesRequest) (workspace.FileListing, error) {
	stub.called("files")
	stub.mu.Lock()
	stub.files = request
	stub.mu.Unlock()
	size := int64(42)
	return workspace.FileListing{Entries: []workspace.FileEntry{{
		Path: "main.go", Name: "main.go", Type: workspace.FileEntryFile,
		SizeBytes: &size, ModifiedAt: "2026-08-12T09:00:00Z",
	}}}, nil
}

func (stub *workspaceServiceStub) Read(_ context.Context, request workspace.ReadRequest) (workspace.FileContent, error) {
	stub.called("read")
	stub.mu.Lock()
	stub.read = request
	stub.mu.Unlock()
	return workspace.FileContent{Path: "main.go", Content: "package main\n", Encoding: "utf-8", TotalLines: 1}, nil
}

func (stub *workspaceServiceStub) inspectionRequests() (workspace.HeadRequest, workspace.SearchRequest, workspace.FilesRequest, workspace.ReadRequest) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.headRequest, stub.search, stub.files, stub.read
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
		if command.input == "/workspaces" {
			host.Shows(t, "project /tmp/project-root")
			host.Shows(t, "2026-08-12T09:00:00Z")
		}
		if service.callCount(command.call) == 0 {
			t.Fatalf("%s did not call %s", command.input, command.call)
		}
		host.Press(input.Esc)
		host.Shows(t, "Ask lyra")
	}
	stop()
}

func TestWorkspaceDiffConsumesModeFormatLimitAndPath(t *testing.T) {
	service := newWorkspaceServiceStub()
	host, stop := runUIWithWorkspaceBackend(t, service, nil)
	host.Shows(t, "Ask lyra")
	host.Type("/diff --base --rows --limit 50 dir with spaces/main.go")
	host.Press(input.Enter)
	host.Shows(t, "Workspace diff")
	host.Shows(t, "base · rows · dir with spaces/main.go")
	request := service.lastDiffRequest()
	if request.Mode != workspace.DiffModeBase || request.Format != workspace.DiffFormatRows ||
		request.Limit != 50 || request.Path != "dir with spaces/main.go" {
		t.Fatalf("diff request = %+v", request)
	}
	stop()
}

func TestWorkspaceDiffOptionsRejectAmbiguousOrInvalidInput(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{
		"--base --worktree",
		"--rows --raw",
		"--limit 0",
		"--limit 10",
		"--unknown",
	} {
		if _, err := parseWorkspaceDiffSelection(argument); err == nil {
			t.Errorf("parseWorkspaceDiffSelection(%q) accepted invalid input", argument)
		}
	}
	selection, err := parseWorkspaceDiffSelection("-- --generated.go")
	if err != nil || selection.path != "--generated.go" {
		t.Fatalf("option terminator = (%+v, %v)", selection, err)
	}
}

func TestWorkspaceCommandsConsumeEveryPublishedQueryOption(t *testing.T) {
	service := newWorkspaceServiceStub()
	host, stop := runUIWithWorkspaceBackend(t, service, nil)
	host.Shows(t, "Ask lyra")

	for _, command := range []struct {
		input string
		show  string
	}{
		{input: "/preview --lines 25 dir with spaces/main.go", show: "File preview"},
		{input: "/grep --path internal --limit 75 transition conflict", show: "Workspace search"},
		{input: "/browse --recursive --ignored --glob *.go dir with spaces", show: "Workspace files"},
		{input: "/read --start 10 --end 20 --max-bytes 4096 dir with spaces/main.go", show: "Workspace file"},
	} {
		host.Type(command.input)
		host.Press(input.Enter)
		host.Shows(t, command.show)
		if command.show == "Workspace files" {
			host.Shows(t, "42 B · 2026-08-12T09:00:00Z")
		}
		host.Press(input.Esc)
		host.Shows(t, "Ask lyra")
	}

	head, search, files, read := service.inspectionRequests()
	if head.Path != "dir with spaces/main.go" || head.Lines != 25 {
		t.Errorf("head request = %+v", head)
	}
	if search.Path != "internal" || search.Limit != 75 || search.Query != "transition conflict" {
		t.Errorf("search request = %+v", search)
	}
	if files.Path != "dir with spaces" || files.Glob != "*.go" || !files.Recursive || !files.IncludeIgnored {
		t.Errorf("files request = %+v", files)
	}
	if read.Path != "dir with spaces/main.go" || read.StartLine != 10 || read.EndLine != 20 || read.MaxBytes != 4096 {
		t.Errorf("read request = %+v", read)
	}
	stop()
}

func TestWorkspaceQueryOptionsFailBeforeStartingARead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parse func(string) error
		input string
	}{
		{name: "preview duplicate", input: "--lines 2 --lines 3 main.go", parse: func(value string) error { _, err := parseWorkspaceHeadSelection(value); return err }},
		{name: "search missing query", input: "--limit 5", parse: func(value string) error { _, err := parseWorkspaceSearchSelection(value); return err }},
		{name: "browse missing glob", input: "--glob", parse: func(value string) error { _, err := parseWorkspaceFilesSelection(value); return err }},
		{name: "read incomplete range", input: "--end 5 main.go", parse: func(value string) error { _, err := parseWorkspaceReadSelection(value); return err }},
		{name: "read reversed range", input: "--start 10 --end 5 main.go", parse: func(value string) error { _, err := parseWorkspaceReadSelection(value); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.parse(test.input); err == nil {
				t.Fatalf("parser accepted %q", test.input)
			}
		})
	}
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
		workspace: "/workspace", watchFiles: true,
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
		workspace: "/workspace", source: source, watchFiles: true,
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

func TestWorkspaceMonitorDoesNotRequestAFileWatchWithoutTheNegotiatedCapability(t *testing.T) {
	t.Parallel()

	source := &runtimeChangeSourceStub{supported: []changefeed.Topic{
		changefeed.FilesChanged, changefeed.SessionsChanged,
	}}
	monitor := runtimeChangeMonitor{
		source: source, repository: changeReaderFunc(func(context.Context, string) ([]workspace.Change, error) {
			return nil, nil
		}),
	}
	if topics := monitor.supportedTopics(); !slices.Equal(topics, []changefeed.Topic{changefeed.SessionsChanged}) {
		t.Fatalf("topics without fileWatch = %v", topics)
	}
	monitor.watchFiles = true
	if topics := monitor.supportedTopics(); !slices.Equal(topics, []changefeed.Topic{changefeed.FilesChanged, changefeed.SessionsChanged}) {
		t.Fatalf("topics with fileWatch = %v", topics)
	}
}

func TestObservedRuntimeResourcesRequireTheirPublishedFeature(t *testing.T) {
	t.Parallel()

	features := map[runtimeprofile.FeatureName]runtimeprofile.Feature{
		runtimeprofile.FeatureGoals:     {Stability: runtimeprofile.Stable},
		runtimeprofile.FeatureSkills:    {Stability: runtimeprofile.Stable},
		runtimeprofile.FeatureMCP:       {Stability: runtimeprofile.Stable},
		runtimeprofile.FeatureSchedules: {Stability: runtimeprofile.Stable},
		runtimeprofile.FeatureKnowledge: {Stability: runtimeprofile.Stable},
	}
	profile := runtimeprofile.Profile{Features: features}
	application := &app{
		runtimeProfile: &profile,
		goals:          new(goalServiceStub),
		skills:         newSkillServiceStub(),
		mcp:            newMCPServiceStub(),
		schedules:      newScheduleServiceStub(),
		knowledge:      newKnowledgeServiceStub(),
		hooks:          &hookServiceStub{},
	}
	if got := application.observedRuntimeResources(); got != (runtimeResourceObservation{hooks: true}) {
		t.Fatalf("resources with disabled features = %+v", got)
	}
	for feature := range features {
		capability := features[feature]
		capability.Enabled = true
		features[feature] = capability
	}
	want := runtimeResourceObservation{
		goals: true, skills: true, mcp: true, schedules: true, knowledge: true, hooks: true,
	}
	if got := application.observedRuntimeResources(); got != want {
		t.Fatalf("resources with enabled features = %+v, want %+v", got, want)
	}

	application.runtimeProfile = nil
	if got := application.observedRuntimeResources(); got != want {
		t.Fatalf("resources without discovery = %+v, want %+v", got, want)
	}
}

var _ workspace.Service = (*workspaceServiceStub)(nil)
var _ changefeed.Source = (*changeSourceStub)(nil)
