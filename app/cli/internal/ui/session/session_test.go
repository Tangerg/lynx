package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func runUI(t *testing.T, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	backend := mock.New()
	backend.Instant = true
	return runUIWith(t, backend, plugins...)
}

func runUIWith(t *testing.T, backend runtime, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	return runUIWithWorkspace(t, backend, "/tmp/lyra-cli-test", plugins...)
}

func runUIWithWorkspace(t *testing.T, backend runtime, workspace string, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: workspace, Plugins: plugins, Host: host,
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

type delayedFirstRuntime struct {
	*mock.Runtime
	starts atomic.Int32
}

type recordingRuntime struct {
	*mock.Runtime
	mu   sync.Mutex
	last client.StartRun
}

type ambiguousControlRuntime struct {
	*mock.Runtime
	mu      sync.Mutex
	starts  int
	resumes int
}

type blockingSessionChangeRuntime struct {
	*mock.Runtime
	creates atomic.Int32
	starts  atomic.Int32

	changeStarted chan struct{}
	releaseChange chan struct{}
	mu            sync.Mutex
	startedIn     string
}

type lostStartRuntime struct {
	*mock.Runtime
	mu           sync.Mutex
	sessionID    string
	canceled     chan struct{}
	canceledOnce sync.Once
}

func (r *lostStartRuntime) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return client.Run{}, err
	}
	r.mu.Lock()
	r.sessionID = input.SessionID
	r.mu.Unlock()
	return client.Run{}, fmt.Errorf("start response lost: %w", client.ErrDisconnected)
}

func (r *lostStartRuntime) CancelRun(ctx context.Context, input client.CancelRun) error {
	if err := r.Runtime.CancelRun(ctx, input); err != nil {
		return err
	}
	r.canceledOnce.Do(func() { close(r.canceled) })
	return nil
}

func (r *lostStartRuntime) startedSession() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}

func (r *blockingSessionChangeRuntime) CreateSession(ctx context.Context, input client.NewSession) (client.Session, error) {
	if r.creates.Add(1) == 1 {
		return r.Runtime.CreateSession(ctx, input)
	}
	close(r.changeStarted)
	select {
	case <-r.releaseChange:
		return r.Runtime.CreateSession(ctx, input)
	case <-ctx.Done():
		return client.Session{}, context.Cause(ctx)
	}
}

func (r *blockingSessionChangeRuntime) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	r.starts.Add(1)
	r.mu.Lock()
	r.startedIn = input.SessionID
	r.mu.Unlock()
	return r.Runtime.StartRun(ctx, input)
}

func (r *blockingSessionChangeRuntime) startedSession() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startedIn
}

func (r *ambiguousControlRuntime) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	run, err := r.Runtime.StartRun(ctx, input)
	if err != nil {
		return client.Run{}, err
	}
	r.mu.Lock()
	r.starts++
	lost := r.starts == 1
	r.mu.Unlock()
	if lost {
		return client.Run{}, fmt.Errorf("lost start response: %w", client.ErrDisconnected)
	}
	return run, nil
}

func (r *ambiguousControlRuntime) ResumeRun(ctx context.Context, input client.ResumeRun) error {
	if err := r.Runtime.ResumeRun(ctx, input); err != nil {
		return err
	}
	r.mu.Lock()
	r.resumes++
	lost := r.resumes == 1
	r.mu.Unlock()
	if lost {
		return fmt.Errorf("lost resume response: %w", client.ErrDisconnected)
	}
	return nil
}

func (r *ambiguousControlRuntime) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts, r.resumes
}

func (r *recordingRuntime) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	r.mu.Lock()
	r.last = input
	r.mu.Unlock()
	return r.Runtime.StartRun(ctx, input)
}

func (r *recordingRuntime) options() client.RunOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last.Options
}

func (r *recordingRuntime) startInput() client.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	input := r.last
	input.Message = cloneMessage(input.Message)
	return input
}

func (r *delayedFirstRuntime) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if r.starts.Add(1) == 1 {
		<-ctx.Done()
		return client.Run{}, context.Cause(ctx)
	}
	return r.Runtime.StartRun(ctx, input)
}

func TestMockConversationStreamsReviewsAndCompletes(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")

	host.Type("why is the cache test flaky?")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Shows(t, "How should lyra proceed?")
	host.Shows(t, "cache_test.go")

	host.Press(input.Enter)
	host.Shows(t, "complete")
	host.Shows(t, "Ran the test 50 times")
	host.Hides(t, "Tool approval")

	// A parked stream may finish at the same moment its continuation starts.
	// The second run proves that the retired stream cannot settle the new one.
	host.Type("run the analysis again")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Hides(t, "complete")
	host.Press(input.Esc)
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")
	host.Hides(t, "failed:")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestInteractiveRunRecoversTransportFaultsWithoutDuplicatingTranscript(t *testing.T) {
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate, mock.FaultGap} {
		t.Run(string(fault), func(t *testing.T) {
			backend := mock.New()
			backend.Instant = true
			backend.Script = stableCompletedScript
			backend.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			host, stop := runUIWith(t, backend)
			host.Shows(t, "Ask lyra")
			host.Type("recover the stream")
			host.Press(input.Enter)
			host.Shows(t, "stable answer")
			host.Shows(t, "complete")
			host.Hides(t, "failed:")
			if count := strings.Count(host.Frame(), "stable answer"); count != 1 {
				t.Fatalf("stable answer appears %d times:\n%s", count, host.Frame())
			}
			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func TestInteractiveRunRecoversAmbiguousControlResponses(t *testing.T) {
	base := mock.New()
	base.Instant = true
	backend := &ambiguousControlRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("recover ambiguous controls")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	host.Hides(t, "failed:")
	starts, resumes := backend.counts()
	if starts != 2 || resumes != 2 {
		t.Fatalf("control calls = start %d, resume %d; want one retry each", starts, resumes)
	}
	if count := strings.Count(host.Frame(), "Replaced the sleep"); count != 1 {
		t.Fatalf("assistant result appears %d times:\n%s", count, host.Frame())
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestClosingTheTerminalCancelsTheOwnedRuntimeRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("keep running until the terminal closes")
	host.Press(input.Enter)
	host.Shows(t, "keep running until")
	started := backend.startInput()
	if started.SessionID == "" {
		t.Fatal("run did not start")
	}
	stop()

	snapshot, err := base.GetSession(t.Context(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("terminal close left an active run: %+v", snapshot.Active)
	}
	finished, ok := snapshot.Events[len(snapshot.Events)-1].Event.(client.RunFinished)
	if !ok || finished.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled run", snapshot.Events[len(snapshot.Events)-1])
	}
}

func TestExhaustedInteractiveStartRetriesCancelTheAcceptedRequest(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		}}
	}
	backend := &lostStartRuntime{Runtime: base, canceled: make(chan struct{})}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("lose all start responses")
	host.Press(input.Enter)
	host.Shows(t, "failed:")
	select {
	case <-backend.canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted start was not canceled after retry exhaustion")
	}

	snapshot, err := base.GetSession(t.Context(), backend.startedSession())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("retry exhaustion left an active run: %+v", snapshot.Active)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestInteractiveRunRejectsConflictingReplay(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	backend.Faults = []mock.SubscriptionFault{{Kind: mock.FaultConflict, After: 1}}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("detect a conflict")
	host.Press(input.Enter)
	host.Shows(t, "failed:")
	host.Shows(t, "event identity conflict")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func stableCompletedScript(string) mock.Script {
	return mock.Script{Prelude: []mock.Step{
		{Event: client.BlockCompleted{Block: client.Block{ID: "answer", Kind: client.BlockAssistant, Text: "stable answer"}}},
		{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
	}}
}

func TestDenyingApprovalIsAProductResult(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("do not change anything without asking")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")

	host.Press(input.Esc)
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSlashCompletionAndTranscriptSearchUseRegisteredCommands(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/he")
	host.Shows(t, "show commands available")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Shows(t, "/find")

	host.Type("/find commands")
	host.Press(input.Enter)
	host.Shows(t, "match(es) for")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestAPluginCanAddACommandWithoutChangingTheShell(t *testing.T) {
	var loads atomic.Int32
	plugin := extensions.Plugin{ID: "test.greeting", Version: "1.0.0", APIVersion: extensions.HostAPIVersion, Capabilities: []extensions.Capability{SlashCommands.Capability()}, Setup: func(scope *extensions.Scope) error {
		loads.Add(1)
		_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
			Name: "hello", Title: "run a contributed command",
			Execute: func(context.Context, CommandRequest) (CommandResult, error) {
				return CommandResult{Message: fmt.Sprintf("hello from plugin load %d", loads.Load())}, nil
			},
		}, extensions.Contribution{})
		return err
	}}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 1")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "loaded   test.greeting@1.0.0")
	host.Type("/reload test.greeting")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "reloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 2")
	host.Type("/unload test.greeting")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Shows(t, "unknown command: /hello")
	host.Type("/reload test.greeting")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "reloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 3")
	if got := loads.Load(); got != 3 {
		t.Fatalf("plugin setup ran %d times, want 3", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestAsynchronousPluginCommandKeepsTheTerminalResponsive(t *testing.T) {
	release := make(chan struct{})
	plugin := extensions.Plugin{
		ID: "test.async", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Name: "slow", Title: "complete asynchronously",
				Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
					select {
					case <-release:
						return CommandResult{Message: "async command complete"}, nil
					case <-ctx.Done():
						return CommandResult{}, context.Cause(ctx)
					}
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/slow")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "running /slow")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "test.async@1.0.0")
	close(release)
	host.Shows(t, "async command complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestPluginCommandPanicBecomesAnError(t *testing.T) {
	_, err := executeCommandSafely(t.Context(), SlashCommand{
		Name:    "boom",
		Execute: func(context.Context, CommandRequest) (CommandResult, error) { panic("command boom") },
	}, CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "command boom") {
		t.Fatalf("command panic error = %v", err)
	}
}

func TestUnloadingPluginCancelsItsInFlightCommand(t *testing.T) {
	canceled := make(chan struct{}, 1)
	plugin := extensions.Plugin{
		ID: "test.cancel", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Name: "wait", Title: "wait until unloaded",
				Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
					<-ctx.Done()
					canceled <- struct{}{}
					return CommandResult{}, context.Cause(ctx)
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/wait")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "running /wait")
	host.Type("/unload test.cancel")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.cancel")
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("plugin command was not canceled on unload")
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestUnloadingAPluginLeavesIndependentCommandsRunning(t *testing.T) {
	canceledA := make(chan struct{}, 1)
	canceledB := make(chan struct{}, 1)
	releaseB := make(chan struct{})
	commandPlugin := func(id, name string, canceled chan<- struct{}, release <-chan struct{}) extensions.Plugin {
		return extensions.Plugin{
			ID: id, Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
			Capabilities: []extensions.Capability{SlashCommands.Capability()},
			Setup: func(scope *extensions.Scope) error {
				_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
					Name: name, Title: "wait for completion",
					Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
						select {
						case <-release:
							return CommandResult{Message: name + " complete"}, nil
						case <-ctx.Done():
							canceled <- struct{}{}
							return CommandResult{}, context.Cause(ctx)
						}
					},
				}, extensions.Contribution{})
				return err
			},
		}
	}
	pluginA := commandPlugin("test.cancel-a", "wait-a", canceledA, nil)
	pluginB := commandPlugin("test.cancel-b", "wait-b", canceledB, releaseB)
	host, stop := runUI(t, pluginA, pluginB)
	host.Shows(t, "Ask lyra")
	for _, command := range []string{"/wait-a", "/wait-b"} {
		host.Type(command)
		host.Press(input.Enter)
		host.Press(input.Enter)
		host.Shows(t, "running "+command)
	}
	host.Type("/unload test.cancel-a")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.cancel-a")
	select {
	case <-canceledA:
	case <-time.After(time.Second):
		t.Fatal("unloaded plugin command was not canceled")
	}
	select {
	case <-canceledB:
		t.Fatal("independent plugin command was canceled")
	default:
	}
	close(releaseB)
	host.Shows(t, "wait-b complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestPluginCommandCannotShadowABuiltin(t *testing.T) {
	plugin := extensions.Plugin{
		ID: "test.shadow", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Name: "help", Title: "shadow help",
				Execute: func(context.Context, CommandRequest) (CommandResult, error) {
					return CommandResult{Message: "shadowed builtin"}, nil
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/help")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Hides(t, "shadowed builtin")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionPickerRestoresHistoryAndLifecycleCommandsSwitchCleanly(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions")
	host.Type("Flaky cache")
	host.Shows(t, "Flaky cache expiry test")
	host.Press(input.Enter)
	host.Hides(t, "search sessions")
	host.Shows(t, "The fixed sleep races the janitor")

	host.Type("/rename Restored cache investigation")
	host.Press(input.Enter)
	host.Shows(t, "renamed session to Restored cache investigation")

	host.Type("/fork Safe alternative")
	host.Press(input.Enter)
	host.Shows(t, "session · Safe alternative")
	host.Shows(t, "The fixed sleep races the janitor")

	host.Type("/new")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionChangeOwnsTheComposerUntilItsSnapshotIsInstalled(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = stableCompletedScript
	backend := &blockingSessionChangeRuntime{
		Runtime:       base,
		changeStarted: make(chan struct{}),
		releaseChange: make(chan struct{}),
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("/new")
	host.Press(input.Enter)
	host.Press(input.Enter)
	select {
	case <-backend.changeStarted:
	case <-time.After(time.Second):
		t.Fatal("session creation did not start")
	}

	host.Type("do not orphan this prompt")
	host.Press(input.Enter)
	host.Shows(t, "wait for the current session change")
	if calls := backend.starts.Load(); calls != 0 {
		t.Fatalf("prompt started %d run(s) before the session change settled", calls)
	}
	close(backend.releaseChange)
	host.Shows(t, "session · Untitled session")
	newSession := firstRuntimeSession(t, base)

	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	if calls := backend.starts.Load(); calls != 1 {
		t.Fatalf("prompt started %d runs after the session change", calls)
	}
	if got := backend.startedSession(); got != newSession {
		t.Fatalf("prompt started in %q, want newly installed session %q", got, newSession)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionSwitchRebindsWorkspaceAttachmentsAndDropsOldChips(t *testing.T) {
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstWorkspace, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondWorkspace, "special.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := mock.New()
	backend.Instant = true
	if _, err := backend.CreateSession(t.Context(), client.NewSession{Title: "Workspace B", Workspace: secondWorkspace}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithWorkspace(t, backend, firstWorkspace)
	host.Shows(t, "Ask lyra")
	host.Type("@old")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached old.txt")

	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions")
	host.Type("Workspace B")
	host.Shows(t, "Workspace B")
	host.Press(input.Enter)
	host.Shows(t, "session · Workspace B")

	host.Type("/attachments")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "the composer has no attachments")
	host.Type("@special")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached special.txt")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func firstRuntimeSession(t *testing.T, runtime client.SessionCatalog) string {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), client.SessionQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("latest session = %+v, %v", page, err)
	}
	return page.Items[0].ID
}

func TestModelModePermissionAndEffortApplyToTheNextRun(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	host, stop := runUIWith(t, backend)
	host.Shows(t, "runtime default · medium · build · ask")

	host.Type("/model")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "Models")
	host.Type("Deep")
	host.Shows(t, "Mock Deep")
	host.Press(input.Enter)
	host.Shows(t, "model · Mock Deep")

	host.Send(input.Key{Code: input.Tab, Mods: input.Shift})
	host.Shows(t, "mode · plan")
	host.Type("/permissions")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "Permissions")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "permissions · read-only")
	host.Type("/effort max")
	host.Press(input.Enter)
	host.Shows(t, "effort · max")
	host.Type("/clear")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "cleared")
	host.Shows(t, "mock-deep · max · plan · read-only")

	host.Type("use these options")
	host.Press(input.Enter)
	host.Shows(t, "How should lyra proceed?")
	host.Press(input.Esc)
	host.Shows(t, "complete")
	if got := backend.options(); got.Model != "mock-deep" || got.Mode != client.ModePlan || got.Permission != client.PermissionReadOnly || got.Effort != "max" {
		t.Fatalf("StartRun options = %+v", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestRunRejectsAnUnresolvableAttachmentWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "loop")
	if err := os.Symlink(workspace, workspace); err != nil {
		t.Fatal(err)
	}
	err := Run(t.Context(), Config{
		Runtime:   mock.New(),
		Workspace: workspace,
		Host:      programtest.New(t, 80, 24),
		Settings:  settings.Default(),
	})
	if err == nil || !strings.Contains(err.Error(), "session attachments") {
		t.Fatalf("Run error = %v, want attachment workspace failure", err)
	}
}

func TestStatusFormatsTheMinimumTokenCount(t *testing.T) {
	if got := thousands(-1 << 63); got != "-9,223,372,036,854,775,808" {
		t.Fatalf("thousands(MinInt64) = %q", got)
	}
}

func TestQuestionFormSubmitsTypedAnswerAndCanCancel(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan client.QuestionAnswer, 2)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interaction: client.Question{
				InterruptID: "question_1", Title: "Choose a strategy", Detail: "One short decision",
				Fields: []client.QuestionField{{
					ID: "strategy", Label: "Strategy", Kind: client.QuestionSingle, Required: true,
					Options: []client.QuestionOption{{Value: "safe", Label: "Safe", Recommended: true}, {Value: "fast", Label: "Fast"}},
				}},
			},
			Continue: func(answer client.Answer) []mock.Step {
				answers <- answer.(client.QuestionAnswer)
				return []mock.Step{{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("ask me")
	host.Press(input.Enter)
	host.Shows(t, "Choose a strategy")
	host.Shows(t, "Safe (recommended)")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if answer := <-answers; answer.Canceled || len(answer.Values["strategy"]) != 1 || answer.Values["strategy"][0] != "safe" {
		t.Fatalf("submitted answer = %+v", answer)
	}

	host.Type("ask again")
	host.Press(input.Enter)
	host.Shows(t, "Choose a strategy")
	host.Press(input.Esc)
	host.Shows(t, "complete")
	if answer := <-answers; !answer.Canceled {
		t.Fatalf("canceled answer = %+v", answer)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalRememberScopeAppliesToLaterRuns(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("first edit")
	host.Press(input.Enter)
	host.Shows(t, "How should lyra proceed?")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "complete")

	host.Type("second edit")
	host.Press(input.Enter)
	host.Shows(t, "Applied remembered approval rule")
	host.Shows(t, "complete")
	host.Hides(t, "How should lyra proceed?")
	rules, err := backend.ListApprovalRules(t.Context())
	if err != nil || len(rules) != 1 || rules[0].Scope != client.RememberSession {
		t.Fatalf("remembered rules = %+v, %v", rules, err)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCommandPaletteSearchAndDetailShortcutsAreReachable(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("status")
	host.Shows(t, "/status")
	host.Press(input.Enter)
	host.Shows(t, "runtime options")

	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Ctrl})
	host.Shows(t, "Find in the live transcript")
	host.Type("model")
	host.Press(input.Enter)
	host.Shows(t, "match(es) for")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "tool details expanded")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "tool details collapsed")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestWorkspaceFileCompletionCreatesAtomicAttachments(t *testing.T) {
	workspace := t.TempDir()
	path := workspace + "/cache_test.go"
	if err := os.WriteFile(path, []byte("package cache\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("@cache")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached cache_test.go")

	// Commands operate on, but do not accidentally submit, staged attachments.
	host.Type("/attachments")
	host.Press(input.Enter)
	host.Shows(t, "attachments")
	host.Shows(t, "cache_test.go · text/")
	host.Type("/detach all")
	host.Press(input.Enter)
	host.Shows(t, "removed all attachments")

	host.Type("/attach cache_test.go")
	host.Press(input.Enter)
	host.Shows(t, "attached cache_test.go")
	host.Type("inspect this file")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	started := backend.startInput()
	if started.Message.Text != "inspect this file" || len(started.Message.Attachments) != 1 {
		t.Fatalf("start message = %+v", started.Message)
	}
	canonical, _ := filepath.EvalSymlinks(path)
	if got := started.Message.Attachments[0]; got.Path != canonical || got.Kind != client.AttachmentText {
		t.Fatalf("attachment = %+v", got)
	}

	// Semantic prompt history restores the chip, not just its visible @label.
	host.Send(input.Key{Code: input.Up, Mods: input.Alt})
	host.Shows(t, "@cache_test.go")
	host.Shows(t, "inspect this file")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestToolKindsRenderLiveAndDetailToggleChangesTheTranscript(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		zero := 0
		return mock.Script{Prelude: []mock.Step{
			{Event: client.BlockStarted{Block: client.Block{ID: "shell", Kind: client.BlockTool, Tool: &client.ToolCall{
				Kind: client.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: client.ToolRunning,
			}}}},
			{Event: client.BlockCompleted{Block: client.Block{ID: "shell", Kind: client.BlockTool, Tool: &client.ToolCall{
				Kind: client.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: client.ToolOK,
				Output: "SHELL_DETAIL_OK\nsecond line", ExitCode: &zero,
			}}}},
			{Event: client.BlockCompleted{Block: client.Block{ID: "edit", Kind: client.BlockTool, Tool: &client.ToolCall{
				Kind: client.ToolEdit, Name: "provider.patch", Path: "internal/cache.go", Summary: "update cache", Status: client.ToolOK,
				Diff: "--- a/internal/cache.go\n+++ b/internal/cache.go\n@@ -1,2 +1,2 @@\n package cache\n-oldTicker()\n+newSweepSignal()\n",
			}}}},
			{Event: client.BlockCompleted{Block: client.Block{ID: "search", Kind: client.BlockTool, Tool: &client.ToolCall{
				Kind: client.ToolSearch, Name: "provider.grep", Query: "cache expiry", Summary: "find expiry", Status: client.ToolOK,
				Output: "internal/cache.go:22\ninternal/cache_test.go:18",
			}}}},
			{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}},
		},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("show tool views")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	host.Shows(t, "edit · internal/cache.go")
	host.Shows(t, "search · cache expiry")
	host.Hides(t, "SHELL_DETAIL_OK")
	host.Hides(t, "newSweepSignal")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "SHELL_DETAIL_OK")
	host.Shows(t, "newSweepSignal")
	host.Shows(t, "internal/cache_test.go:18")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Hides(t, "SHELL_DETAIL_OK")
	host.Hides(t, "newSweepSignal")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelBeforeRunIdentityDoesNotBlockTheNextRun(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	host, stop := runUIWith(t, &delayedFirstRuntime{Runtime: backend})
	host.Shows(t, "Ask lyra")
	host.Type("first request waits before returning a stream")
	host.Press(input.Enter)
	host.Shows(t, "starting run")
	host.Send(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	host.Shows(t, "canceled")

	host.Type("second request can start")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Esc)
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalRemainsUsableAtRepresentativeWidths(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "narrow", width: 44, height: 18},
		{name: "wide", width: 120, height: 32},
	} {
		t.Run(size.name, func(t *testing.T) {
			host, stop := runUI(t)
			host.Shows(t, "Ask lyra")
			if !host.Resize(size.width, size.height) {
				t.Fatalf("resize to %dx%d was refused", size.width, size.height)
			}
			host.Type("review this at the current terminal width")
			host.Press(input.Enter)
			host.Shows(t, "How should lyra proceed?")
			host.Press(input.Esc)
			host.Shows(t, "Left the file alone")
			host.Shows(t, "complete")

			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func TestTerminalSurvivesExtremeResizeAndRemainsInteractive(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	for _, size := range []struct{ width, height int }{{20, 8}, {200, 60}, {32, 10}, {96, 28}} {
		if !host.Resize(size.width, size.height) {
			t.Fatalf("resize to %dx%d was refused", size.width, size.height)
		}
		if !host.Repaint() {
			t.Fatalf("repaint at %dx%d was refused", size.width, size.height)
		}
	}
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "terminal.core@1.0.0")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}
