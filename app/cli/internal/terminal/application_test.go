package terminal

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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func runUI(t *testing.T, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	backend := mock.New()
	backend.Instant = true
	return runUIWith(t, backend, plugins...)
}

func runUIWith(t *testing.T, backend agent.Runtime, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	return runUIWithWorkspace(t, backend, "/tmp/lyra-cli-test", plugins...)
}

func runUIWithWorkspace(t *testing.T, backend agent.Runtime, workspace string, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	return runUIConfigured(t, backend, workspace, nil, plugins...)
}

func runUIWithSettings(t *testing.T, backend agent.Runtime, configured settings.Config) (*programtest.Host, func()) {
	t.Helper()
	return runUIConfigured(t, backend, "/tmp/lyra-cli-test", &configured)
}

func runUIConfigured(t *testing.T, backend agent.Runtime, workspace string, configured *settings.Config, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: workspace, Plugins: plugins, Host: host, Settings: configured,
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

func runUIForSession(t *testing.T, backend agent.Runtime, sessionID string) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: backend, SessionID: sessionID, Host: host})
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

	mu     sync.Mutex
	last   agent.StartRun
	inputs []agent.StartRun
	starts int
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

func (r *blockingSessionChangeRuntime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	if r.creates.Add(1) == 1 {
		return r.Runtime.CreateSession(ctx, input)
	}
	close(r.changeStarted)
	select {
	case <-r.releaseChange:
		return r.Runtime.CreateSession(ctx, input)
	case <-ctx.Done():
		return agent.Session{}, context.Cause(ctx)
	}
}

func (r *blockingSessionChangeRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
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

func (r *recordingRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	r.mu.Lock()
	r.last = cloneStartRun(input)
	r.inputs = append(r.inputs, cloneStartRun(input))
	r.starts++
	r.mu.Unlock()
	return r.Runtime.StartRun(ctx, input)
}

func (r *recordingRuntime) startCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts
}

func (r *recordingRuntime) options() agent.RunOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last.Options
}

func (r *recordingRuntime) startInput() agent.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	input := r.last
	input.Message = input.Message.Clone()
	return input
}

func (r *recordingRuntime) startInputs() []agent.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	inputs := make([]agent.StartRun, len(r.inputs))
	for index, input := range r.inputs {
		inputs[index] = cloneStartRun(input)
	}
	return inputs
}

func cloneStartRun(input agent.StartRun) agent.StartRun {
	input.Message = input.Message.Clone()
	return input
}

func (r *delayedFirstRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	if r.starts.Add(1) == 1 {
		<-ctx.Done()
		return agent.SegmentStream{}, context.Cause(ctx)
	}
	return r.Runtime.StartRun(ctx, input)
}

func TestMockConversationStreamsApprovalAndCompletes(t *testing.T) {
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

func TestShiftEnterInsertsANewlineWithoutSubmitting(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Shows(t, "shift+enter")
	host.Type("first line")
	host.Send(input.Key{Code: input.Enter, Mods: input.Shift})
	host.Type("second line")
	host.Press(input.Enter)
	host.Shows(t, "complete")

	if got := backend.startInput().Message.Text; got != "first line\nsecond line" {
		t.Fatalf("submitted text = %q, want a two-line prompt", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestConfiguredKeySequencesDriveApplicationActions(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g s"}
	configured.Keys[settings.ActionSend] = []string{"g g"}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithSettings(t, backend, configured)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Send(input.Key{Code: input.Character, Rune: 's'})
	host.Shows(t, "Sessions")
	host.Press(input.Esc)
	host.Hides(t, "Sessions")

	host.Type("sequence submit")
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Shows(t, "complete")
	if got := backend.startInput().Message.Text; got != "sequence submit" {
		t.Fatalf("submitted text = %q, want the configured key sequence to submit the draft", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestConfiguredPrefixesRespectModalOwnershipAndResolveAfterTimeout(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g"}
	configured.Keys[settings.ActionShortcuts] = []string{"g s"}
	host, stop := runUIWithSettings(t, mock.New(), configured)
	host.Shows(t, "Ask lyra")

	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Ctrl})
	host.Shows(t, "Find in the live transcript")
	host.Type("g sequence")
	host.Shows(t, "g sequence")
	host.Hides(t, "Sessions")
	host.Press(input.Esc)
	host.Hides(t, "Find in the live transcript")

	// The exact g binding is also a prefix of g s, so it resolves through the
	// application clock only after the configured sequence timeout expires.
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Shows(t, "Sessions")
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestTranscriptFocusDoesNotSubmitAndTypingReturnsToPrompt(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(prompt string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: "focused answer · " + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("first prompt")
	host.Press(input.Enter)
	host.Shows(t, "focused answer · first prompt")
	host.Shows(t, "complete")

	host.Press(input.Tab)
	host.Shows(t, "select prev")
	host.Type("?")
	host.Shows(t, "Commands")
	host.Press(input.Esc)
	host.Hides(t, "Commands")
	host.Press(input.Enter)
	if got := backend.startCount(); got != 1 {
		t.Fatalf("Enter with transcript focus started %d runs, want 1", got)
	}

	host.Type("second prompt")
	host.Shows(t, "second prompt")
	host.Press(input.Enter)
	host.Shows(t, "focused answer · second prompt")
	if got := backend.startCount(); got != 2 {
		t.Fatalf("typing from transcript focus started %d runs, want 2", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCtrlCClearsTheDraftBeforeCancelingAnActiveRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("start a long run")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("UNSENT_CTRL_C_DRAFT")
	host.Shows(t, "UNSENT_CTRL_C_DRAFT")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Hides(t, "UNSENT_CTRL_C_DRAFT")
	host.Shows(t, "draft cleared; repeat ctrl+c to cancel")
	started := backend.startInput()
	snapshot, err := base.GetSession(t.Context(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); !active {
		t.Fatal("clear-first Ctrl+C canceled the active run")
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "canceled")
	stop()
}

func TestEscapeCancelsAnActiveRunWithoutDiscardingTheDraft(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, base)
	host.Shows(t, "Ask lyra")
	host.Type("start another long run")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("PRESERVED_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Shows(t, "canceled")
	host.Shows(t, "PRESERVED_ESCAPE_DRAFT")
	stop()
}

func TestDoubleEscapeClearsAnIdleDraftAndKeepsItInHistory(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("RECOVERABLE_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Shows(t, "press Esc again to clear the draft")
	host.Shows(t, "RECOVERABLE_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Hides(t, "RECOVERABLE_ESCAPE_DRAFT")
	host.Send(input.Key{Code: input.Up, Mods: input.Alt})
	host.Shows(t, "RECOVERABLE_ESCAPE_DRAFT")
	stop()
}

func TestQuitRequiresAConfirmingSecondPress(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: "/tmp/lyra-cli-test", Host: host,
		})
	}()

	host.Shows(t, "Ask lyra")
	quit := input.Key{Code: input.Character, Rune: 'q', Mods: input.Ctrl}
	host.Send(quit)
	host.Shows(t, "repeat ctrl+q or ctrl+d to quit")
	select {
	case err := <-done:
		t.Fatalf("first quit press stopped the app: %v", err)
	default:
	}

	host.Send(quit)
	wait, stopWaiting := context.WithTimeout(t.Context(), 2*time.Second)
	defer stopWaiting()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("confirmed quit returned an error: %v", err)
		}
	case <-wait.Done():
		t.Fatal("confirming quit press did not stop the app")
	}
}

func TestInteractiveRunRecoversTransportFaultsWithoutDuplicatingTranscript(t *testing.T) {
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate} {
		t.Run(string(fault), func(t *testing.T) {
			backend := mock.New()
			backend.Instant = fault == mock.FaultDuplicate
			backend.Script = stableCompletedScript
			backend.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			host, stop := runUIWith(t, backend)
			host.Shows(t, "Ask lyra")
			host.Type("recover the stream")
			host.Press(input.Enter)
			assertStableCompletedTranscript(t, host)
			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func TestInteractiveRunColdRecoversWhenTheDroppedSegmentAlreadyFinished(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	backend.Faults = []mock.SubscriptionFault{{Kind: mock.FaultDisconnect, After: 1}}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("recover from cold state")
	host.Press(input.Enter)
	assertStableCompletedTranscript(t, host)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func assertStableCompletedTranscript(t *testing.T, host *programtest.Host) {
	t.Helper()
	host.Shows(t, "complete")
	host.Hides(t, "failed:")
	host.Shows(t, "stable answer")
	if count := strings.Count(host.Frame(), "stable answer"); count != 1 {
		t.Fatalf("stable answer appears %d times:\n%s", count, host.Frame())
	}
}

func TestClosingTheTerminalCancelsTheOwnedRuntimeRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
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
	if _, active := snapshot.ActiveRun(); active {
		t.Fatalf("terminal close left an active run: %+v", snapshot.Runs)
	}
	if snapshot.Session.Status != agent.SessionIdle {
		t.Fatalf("session status = %s, want idle", snapshot.Session.Status)
	}
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
		{Delay: 30 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "stable answer"}}},
		{Delay: 100 * time.Millisecond, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
	}}
}

func TestDenyingApprovalIsAProductResult(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("do not change anything without asking")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSlashCompletionHelpAndTranscriptSearchUseRegisteredCommands(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/he")
	host.Shows(t, "show commands available")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Shows(t, "/find")
	host.Press(input.Esc)
	host.Hides(t, "Commands")

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
	host.Hides(t, "The fixed sleep races the janitor")

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
	if _, err := backend.CreateSession(t.Context(), agent.CreateSession{Title: "Workspace B", Workspace: secondWorkspace}); err != nil {
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

func firstRuntimeSession(t *testing.T, runtime agent.SessionCatalog) string {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("latest session = %+v, %v", page, err)
	}
	return page.Items[0].ID
}

func TestProviderQualifiedModelAndLimitsApplyToTheNextRun(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	configured := settings.Default()
	configured.Run.MaxSteps = 42
	configured.Run.MaxBudgetUSD = 2.5
	host, stop := runUIWithSettings(t, backend, configured)
	host.Shows(t, settings.DefaultProvider+"/"+settings.DefaultModel)

	host.Type("/model")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "Models")
	host.Type("Deep")
	host.Shows(t, "Synthetic Deep")
	host.Press(input.Enter)
	host.Shows(t, "model · synthetic/deep")

	host.Shows(t, "synthetic/deep")

	host.Type("use these options")
	host.Press(input.Enter)
	host.Shows(t, "How should lyra proceed?")
	host.Press(input.Esc)
	host.Shows(t, "complete")
	if got := backend.options(); got.Provider != "synthetic" || got.Model != "deep" || got.Limits.MaxSteps != 42 || got.Limits.MaxBudgetUSD != 2.5 {
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
		Settings:  new(settings.Default()),
	})
	if err == nil || !strings.Contains(err.Error(), "session attachments") {
		t.Fatalf("Run error = %v, want attachment workspace failure", err)
	}
}

func TestPrepareSessionDistinguishesDefaultsFromExplicitFalseValues(t *testing.T) {
	configured := settings.Default()
	configured.UI.Mouse = false
	prepared, err := prepareSession(t.Context(), Config{
		Runtime: mock.New(), Workspace: t.TempDir(), Settings: new(configured),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.settings.UI.Mouse {
		t.Fatal("explicit mouse=false was replaced by the default")
	}

	defaults, err := prepareSession(t.Context(), Config{Runtime: mock.New(), Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !defaults.settings.UI.Mouse || defaults.settings.Keys == nil {
		t.Fatalf("nil settings did not produce complete defaults: %+v", defaults.settings)
	}
}

func TestFormatThousandsHandlesMinimumInt64(t *testing.T) {
	if got := formatThousands(-1 << 63); got != "-9,223,372,036,854,775,808" {
		t.Fatalf("formatThousands(MinInt64) = %q", got)
	}
}

func TestQuestionFormSubmitsTypedAnswerAndCanCancel(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 2)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "question_1", Title: "Choose a strategy", Detail: "One short decision",
				Fields: []agent.QuestionField{{
					Header: "Strategy", Prompt: "Choose a strategy", Kind: agent.QuestionSingle,
					Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}},
				}},
			}},
			Continue: func(answerSet []agent.InterruptAnswer) []mock.Step {
				answers <- answerSet[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("ask me")
	host.Press(input.Enter)
	host.Shows(t, "Choose a strategy")
	host.Shows(t, "Safe")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if answer := <-answers; len(answer.Values) != 1 || len(answer.Values[0]) != 1 || answer.Values[0][0] != "Safe" {
		t.Fatalf("submitted answer = %+v", answer)
	}

	host.Type("ask again")
	host.Press(input.Enter)
	host.Shows(t, "Choose a strategy")
	host.Press(input.Esc)
	host.Shows(t, "canceled")

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
	sessionID := firstRuntimeSession(t, backend)
	rules, err := backend.ListApprovalRules(t.Context(), sessionID)
	if err != nil || len(rules) != 1 || rules[0].Scope != agent.RememberSession {
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

func TestShortcutGuideReflectsActiveBindings(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	host.Shows(t, "Shortcuts")
	host.Shows(t, "ctrl+x")
	host.Shows(t, "open this shortcut guide")
	host.Shows(t, "shift+enter / alt+enter")
	host.Shows(t, "insert a newline")
	host.Press(input.End)
	host.Hides(t, "scroll this guide up")
	host.Press(input.Esc)
	host.Hides(t, "open this shortcut guide")

	host.Type("/shortcuts")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "Shortcuts")
	host.Shows(t, "scroll this guide up")
	host.Press(input.Esc)
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
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
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
	if got := started.Message.Attachments[0]; got.Path != canonical || got.Kind != agent.AttachmentText {
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
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: agent.ToolRunning,
			}}}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "SHELL_DETAIL_"}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "OK\nsecond line"}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: agent.ToolOK,
				Output: "SHELL_DETAIL_OK\nsecond line", ExitCode: &zero,
			}}}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "edit", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolEdit, Name: "provider.patch", Path: "internal/cache.go", Summary: "update cache", Status: agent.ToolOK,
				Diff: "--- a/internal/cache.go\n+++ b/internal/cache.go\n@@ -1,2 +1,2 @@\n package cache\n-oldTicker()\n+newSweepSignal()\n",
			}}}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "search", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolSearch, Name: "provider.grep", Query: "cache expiry", Summary: "find expiry", Status: agent.ToolOK,
				Output: "internal/cache.go:22\ninternal/cache_test.go:18",
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
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

func TestRunningToolOutputStreamsIntoAnExpandedTranscript(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		zero := 0
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolRunning,
			}}}},
			{Delay: 100 * time.Millisecond, Event: agent.BlockDelta{BlockID: "shell", Text: "LIVE_TOOL_FIRST\n"}},
			{Delay: 400 * time.Millisecond, Event: agent.BlockDelta{BlockID: "shell", Text: "LIVE_TOOL_SECOND\n"}},
			{Delay: 400 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolOK,
				Output: "LIVE_TOOL_FIRST\nLIVE_TOOL_SECOND\n", ExitCode: &zero,
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("stream tool output")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "LIVE_TOOL_FIRST")
	host.Shows(t, "LIVE_TOOL_SECOND")
	host.Shows(t, "done")
	host.Shows(t, "complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelingARunSettlesItsLiveToolProjection(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "long command", Status: agent.ToolRunning,
			}}}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "PARTIAL_TOOL_OUTPUT\n"}},
			{Delay: time.Hour, Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "long command", Status: agent.ToolOK, Output: "never reached",
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("cancel live tool")
	host.Press(input.Enter)
	host.Shows(t, "$ long command")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "PARTIAL_TOOL_OUTPUT")
	host.Press(input.Esc)
	host.Shows(t, "canceled")
	host.Hides(t, "running")
	host.Hides(t, "never reached")
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
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
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
			backend := mock.New()
			backend.Instant = true
			backend.Script = approvalWidthScript
			host, stop := runUIWith(t, backend)
			host.Shows(t, "Ask lyra")
			if !host.Resize(size.width, size.height) {
				t.Fatalf("resize to %dx%d was refused", size.width, size.height)
			}
			host.Type("review this at the current terminal width")
			host.Press(input.Enter)
			host.Shows(t, "How should lyra proceed?")
			host.Press(input.Esc)
			host.Shows(t, "Approval denied at current width")
			host.Shows(t, "complete")

			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func approvalWidthScript(string) mock.Script {
	return mock.Script{
		Interactions: []agent.Interaction{agent.Approval{
			ItemID: "responsive-approval",
			Title:  "Review the proposed change",
			Detail: "Confirm that the approval form remains usable at this terminal width.",
			Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "responsive-check", Command: "go test ./...", Status: agent.ToolRunning,
			},
		}},
		Continue: func(answers []agent.InterruptAnswer) []mock.Step {
			message := "Approval was not denied at current width"
			if len(answers) == 1 {
				answer, ok := answers[0].Answer.(agent.ApprovalAnswer)
				if ok && answer.Decision == agent.ApprovalDeny {
					message = "Approval denied at current width"
				}
			}
			return []mock.Step{
				{Event: agent.BlockCompleted{Block: agent.Block{ID: "responsive-result", Kind: agent.BlockAssistant, Text: message}}},
				{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}
		},
	}
}

func TestTerminalSurvivesExtremeResizeAndRemainsInteractive(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("resize draft")
	for _, size := range []struct{ width, height int }{{20, 7}, {10, 7}, {200, 60}, {32, 10}, {96, 28}} {
		if !host.Resize(size.width, size.height) {
			t.Fatalf("resize to %dx%d was refused", size.width, size.height)
		}
		if !host.Repaint() {
			t.Fatalf("repaint at %dx%d was refused", size.width, size.height)
		}
	}
	host.Shows(t, "resize draft")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Hides(t, "resize draft")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "terminal.core@1.0.0")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestStreamingRemainsResponsiveThroughAResizeStorm(t *testing.T) {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		steps := []mock.Step{{Event: agent.BlockStarted{Block: agent.Block{
			ID: "stream", Kind: agent.BlockAssistant,
		}}}}
		for range 64 {
			steps = append(steps, mock.Step{
				Delay: 2 * time.Millisecond,
				Event: agent.BlockDelta{BlockID: "stream", Text: "x"},
			})
		}
		steps = append(steps,
			mock.Step{Event: agent.BlockCompleted{Block: agent.Block{
				ID: "stream", Kind: agent.BlockAssistant, Text: "RESIZE_STREAM_COMPLETE",
			}}},
			mock.Step{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		)
		return mock.Script{Prelude: steps}
	}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("stress the viewport")
	host.Press(input.Enter)

	sizes := []struct{ width, height int }{
		{1, 1}, {120, 40}, {8, 3}, {96, 28}, {11, 20}, {200, 60}, {20, 7}, {80, 24},
	}
	for iteration := range 8 {
		for _, size := range sizes {
			if !host.Resize(size.width, size.height) || !host.Repaint() {
				t.Fatalf("resize storm stopped at iteration %d size %dx%d", iteration, size.width, size.height)
			}
		}
	}
	if !host.Resize(96, 28) {
		t.Fatal("could not restore the normal viewport")
	}
	host.Shows(t, "RESIZE_STREAM_COMPLETE")
	host.Shows(t, "complete")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "terminal.core")
	stop()
}

func TestOpeningAnActiveSessionRecoversAStreamWhoseTransientStartPredatesAttachment(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}}},
			{Event: agent.BlockDelta{BlockID: "answer", Text: "provisional"}},
			{Delay: 200 * time.Millisecond, Event: agent.BlockDelta{BlockID: "answer", Text: " preview"}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "RECOVERED_AUTHORITATIVE_ANSWER"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, err := backend.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := backend.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "recover me"}})
	if err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, delta := event.Event.(agent.BlockDelta); delta {
			break
		}
	}

	host, stop := runUIForSession(t, backend, session.ID)
	host.Shows(t, "RECOVERED_AUTHORITATIVE_ANSWER")
	host.Shows(t, "complete")
	stop()
}

func TestOutcomeNotificationMatchesTheRunVerdict(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome agent.Outcome
		want    string
	}{
		{name: "completed", outcome: agent.Outcome{Status: agent.OutcomeCompleted}, want: "lyra run completed"},
		{name: "canceled", outcome: agent.Outcome{Status: agent.OutcomeCanceled}, want: "lyra run canceled"},
		{name: "failed", outcome: agent.Outcome{Status: agent.OutcomeFailed, Error: "boom"}, want: "lyra run failed"},
		{name: "unsettled", outcome: agent.Outcome{}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := outcomeNotification(test.outcome); got != test.want {
				t.Fatalf("outcomeNotification(%+v) = %q, want %q", test.outcome, got, test.want)
			}
		})
	}
}
