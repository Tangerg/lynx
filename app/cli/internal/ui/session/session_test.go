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

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
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
			Run: func(host CommandHost, _ string) error {
				host.SetStatus(fmt.Sprintf("hello from plugin load %d", loads.Load()))
				return nil
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
	err := runCommandSafely(SlashCommand{
		Name: "boom",
		Run:  func(CommandHost, string) error { panic("command boom") },
	}, nil, "")
	if err == nil || !strings.Contains(err.Error(), "command boom") {
		t.Fatalf("command panic error = %v", err)
	}
}

func TestSessionPickerRestoresHistoryAndLifecycleCommandsSwitchCleanly(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/sessions")
	host.Press(input.Enter)
	host.Press(input.Enter)
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

func TestModelModePermissionAndEffortApplyToTheNextRun(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	host, stop := runUIWith(t, backend)
	host.Shows(t, "mock-balanced · medium · build · ask")

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
	host.Shows(t, "starting mock runtime")
	host.Send(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	host.Shows(t, "cancelled")

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
