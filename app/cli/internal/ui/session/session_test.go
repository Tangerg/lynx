package session

import (
	"context"
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
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: "/tmp/lyra-cli-test", Plugins: plugins, Host: host,
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
	host.Shows(t, "Allow this tool call?")
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
	plugin := extensions.Plugin{ID: "test.greeting", Setup: func(scope *extensions.Scope) error {
		_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
			Name: "hello", Title: "run a contributed command",
			Run: func(host CommandHost, _ string) error {
				host.SetStatus("hello from a plugin")
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
	host.Shows(t, "hello from a plugin")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
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
			host.Shows(t, "Allow this tool call?")
			host.Press(input.Esc)
			host.Shows(t, "Left the file alone")
			host.Shows(t, "complete")

			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}
