package sideload

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/ui/session"
)

func writePlugin(t *testing.T, root, directory string, declared manifest, executable string) string {
	t.Helper()
	pluginDirectory := filepath.Join(root, directory)
	if err := os.MkdirAll(pluginDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if executable != "" {
		if err := os.WriteFile(filepath.Join(pluginDirectory, declared.Entry), []byte(executable), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(declared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDirectory, manifestName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return pluginDirectory
}

func validManifest(id string) manifest {
	return manifest{
		SchemaVersion: manifestSchema, ID: id, Version: "1.2.3", APIVersion: extensions.HostAPIVersion,
		Capabilities: []string{"terminal.commands"}, Entry: "plugin",
		Contributes: contributions{Commands: []commandManifest{{Name: "hello", Title: "say hello", Takes: true}}},
	}
}

func TestSourceDiscoversValidPluginsAndIsolatesMalformedNeighbors(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "good", validManifest("test.good"), "not executed")
	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, manifestName), []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}

	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 1 || discovered.Plugins[0].ID != "test.good" {
		t.Fatalf("plugins = %+v", discovered.Plugins)
	}
	if len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "broken") {
		t.Fatalf("issues = %+v", discovered.Issues)
	}
}

func TestManifestEntryCannotEscapePluginDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o755); err != nil {
		t.Fatal(err)
	}
	declared := validManifest("test.escape")
	declared.Entry = "../outside"
	writePlugin(t, root, "escape", declared, "")

	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "unsafe path") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestSideloadedPluginMustDeclareCommandsCapability(t *testing.T) {
	root := t.TempDir()
	declared := validManifest("test.denied")
	declared.Capabilities = []string{}
	writePlugin(t, root, "denied", declared, "not executed")
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	registry := new(extensions.Registry)
	kernel, err := extensions.NewKernel(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Close()
	results, err := kernel.Activate(discovered.Plugins)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Phase != extensions.PluginFailed || !strings.Contains(results[0].Err.Error(), "terminal.commands") {
		t.Fatalf("activation = %+v", results)
	}
	if commands := extensions.Values(registry, session.SlashCommands); len(commands) != 0 {
		t.Fatalf("denied plugin registered commands: %+v", commands)
	}
}

func TestCommandRunnerUsesBoundedJSONProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	declared := validManifest("test.runner")
	script := "#!/bin/sh\nread request\nprintf '{\"protocol\":1,\"message\":\"hello from process\"}'\n"
	writePlugin(t, root, "runner", declared, script)
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil || len(discovered.Issues) != 0 {
		t.Fatalf("discovery = %+v, %v", discovered, err)
	}
	registry := new(extensions.Registry)
	kernel, err := extensions.NewKernel(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer kernel.Close()
	results, err := kernel.Activate(discovered.Plugins)
	if err != nil || len(results) != 1 || results[0].Phase != extensions.PluginLoaded {
		t.Fatalf("activation = %+v, %v", results, err)
	}
	commands := extensions.Values(registry, session.SlashCommands)
	if len(commands) != 1 || commands[0].Execute == nil {
		t.Fatalf("commands = %+v", commands)
	}
	response, err := commands[0].Execute(t.Context(), session.CommandRequest{
		Argument: "world", Workspace: "/tmp/work", SessionID: "session-1",
	})
	if err != nil || response.Message != "hello from process" {
		t.Fatalf("response = %+v, %v", response, err)
	}
}

func TestCommandRunnerHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	slow := filepath.Join(root, "slow")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := commandRunner{pluginID: "test.slow", command: "slow", executable: slow, directory: root, timeout: 20 * time.Millisecond}
	_, err := runner.Execute(t.Context(), session.CommandRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestCommandResponseRejectsMalformedProtocolWithoutProcessTiming(t *testing.T) {
	if _, err := decodeCommandResponse("test.bad", "bad", []byte("not json")); err == nil || !strings.Contains(err.Error(), "decode plugin") {
		t.Fatalf("malformed response error = %v", err)
	}
}

func TestSideloadedCommandRunsEndToEndInTheTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	declared := validManifest("test.e2e")
	declared.Contributes.Commands[0].Takes = false
	script := "#!/bin/sh\nread request\nprintf '{\"protocol\":1,\"message\":\"sideload end to end\"}'\n"
	writePlugin(t, root, "e2e", declared, script)
	backend := mock.New()
	backend.Instant = true
	host := programtest.New(t, 96, 28)
	workspace := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- session.Run(ctx, session.Config{
			Runtime: backend, Workspace: workspace, Host: host,
			PluginSources: []extensions.Source{New([]string{root})},
		})
	}()
	host.Shows(t, "Ask lyra")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "sideload end to end")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("terminal stopped with %v", err)
	}
}
