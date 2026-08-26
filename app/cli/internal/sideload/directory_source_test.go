package sideload

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/terminal"
)

func writePlugin(t *testing.T, root, directory string, declared pluginManifest, executable string) {
	t.Helper()
	pluginDirectory := filepath.Join(root, directory)
	if err := os.MkdirAll(pluginDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	if executable != "" {
		writeExecutable(t, filepath.Join(pluginDirectory, declared.Entry), executable)
	}
	encoded, err := json.Marshal(declared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDirectory, manifestName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func validManifest(id string) pluginManifest {
	return pluginManifest{
		SchemaVersion: manifestSchemaVersion, ID: id, Version: "1.2.3", APIVersion: extensions.HostAPIVersion,
		Capabilities: []string{"terminal.commands"}, Entry: "plugin",
		Contributes: manifestContributions{Commands: []commandManifest{{Name: "hello", Title: "say hello", Arguments: "required"}}},
	}
}

func TestDirectorySourceDiscoversValidPluginsAndIsolatesMalformedNeighbors(t *testing.T) {
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

func TestDirectorySourceDeduplicatesCanonicalPluginDirectories(t *testing.T) {
	root := t.TempDir()
	pluginDirectory := filepath.Join(root, "good")
	writePlugin(t, root, "good", validManifest("test.good"), "not executed")
	discovered, err := New([]string{root, filepath.Join(root, "."), pluginDirectory, pluginDirectory}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Issues) != 0 || len(discovered.Plugins) != 1 || discovered.Plugins[0].ID != "test.good" {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestManifestEntryCannotEscapePluginDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	writeExecutable(t, outside, "outside")
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
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer extensionHost.Close()
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Phase != extensions.PluginFailed || !strings.Contains(results[0].Err.Error(), "terminal.commands") {
		t.Fatalf("activation = %+v", results)
	}
	if commands := registry.Values(terminal.SlashCommands); len(commands) != 0 {
		t.Fatalf("denied plugin registered commands: %+v", commands)
	}
}

func TestSideloadedCommandRejectsAnInvalidArgumentMode(t *testing.T) {
	root := t.TempDir()
	declared := validManifest("test.arguments")
	declared.Contributes.Commands[0].Arguments = "sometimes"
	writePlugin(t, root, "arguments", declared, "not executed")
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "invalid arguments mode") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestExecutableCommandUsesBoundedJSONProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	declared := validManifest("test.runner")
	script := "#!/bin/sh\nread request\nprintf '{\"protocol\":1,\"message\":\"hello from process\"}'\n"
	writePlugin(t, root, "runner", declared, script)
	commands, closeKernel := loadFixtureCommands(t, root)
	defer closeKernel()
	if len(commands) != 1 || commands[0].Execute == nil {
		t.Fatalf("commands = %+v", commands)
	}
	response, err := commands[0].Execute(t.Context(), terminal.CommandRequest{
		Argument: "world", Workspace: "/tmp/work", SessionID: "session-1",
	})
	if err != nil || response.Message != "hello from process" {
		t.Fatalf("response = %+v, %v", response, err)
	}
}

func loadFixtureCommands(t *testing.T, root string) ([]terminal.SlashCommand, func()) {
	t.Helper()
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil || len(discovered.Issues) != 0 {
		t.Fatalf("discovery = %+v, %v", discovered, err)
	}
	registry := new(extensions.Registry)
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil || len(results) != 1 || results[0].Phase != extensions.PluginLoaded {
		_ = extensionHost.Close()
		t.Fatalf("activation = %+v, %v", results, err)
	}
	return registry.Values(terminal.SlashCommands), func() { _ = extensionHost.Close() }
}

func TestExecutableCommandHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	slow := filepath.Join(root, "slow")
	writeExecutable(t, slow, "#!/bin/sh\nsleep 2\n")
	executor := executableCommand{pluginID: "test.slow", command: "slow", executable: slow, directory: root, timeout: 20 * time.Millisecond}
	_, err := executor.Execute(t.Context(), terminal.CommandRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestCommandResponseRejectsMalformedProtocolWithoutProcessTiming(t *testing.T) {
	if _, err := decodeCommandResponse("test.bad", "bad", []byte("not json")); err == nil || !strings.Contains(err.Error(), "decode plugin") {
		t.Fatalf("malformed response error = %v", err)
	}
}

func TestCommandResponseNamesGenericTrailingJSON(t *testing.T) {
	_, err := decodeCommandResponse("test.bad", "bad", []byte(`{"protocol":1,"message":"ok"} {}`))
	if err == nil || !strings.Contains(err.Error(), "input contains multiple JSON values") || strings.Contains(err.Error(), "manifest") {
		t.Fatalf("trailing response error = %v", err)
	}
}

func TestCommandRequestIsBoundedBeforeAProcessStarts(t *testing.T) {
	executor := executableCommand{pluginID: "test.bounds", command: "large", executable: filepath.Join(t.TempDir(), "missing"), timeout: time.Second}
	_, err := executor.Execute(t.Context(), terminal.CommandRequest{Argument: strings.Repeat("x", maxCommandArgumentBytes+1)})
	if err == nil || !strings.Contains(err.Error(), "argument exceeds") || strings.Contains(err.Error(), "executable") {
		t.Fatalf("oversized request error = %v", err)
	}
}

func TestCommandEnvironmentUsesAnExplicitAllowlist(t *testing.T) {
	t.Setenv("LYRA_TEST_SECRET", "must-not-leak")
	t.Setenv("PATH", "/safe/bin")
	environment := commandEnvironment("test.safe", "hello")
	if !slices.Contains(environment, "PATH=/safe/bin") || !slices.Contains(environment, "LYRA_PLUGIN_ID=test.safe") || !slices.Contains(environment, "LYRA_PLUGIN_COMMAND=hello") {
		t.Fatalf("command environment = %v", environment)
	}
	if slices.ContainsFunc(environment, func(value string) bool { return strings.HasPrefix(value, "LYRA_TEST_SECRET=") }) {
		t.Fatalf("secret leaked into command environment: %v", environment)
	}
}
