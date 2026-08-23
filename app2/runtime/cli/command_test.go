package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/runtimehost"
)

func TestServeMapsConfigEnvAndFlagsWithoutGlobalState(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "runtime.yaml")
	if err := os.WriteFile(configPath, []byte("listen: 127.0.0.1:18001\nserverName: from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("LYRA2_LISTEN", "127.0.0.1:18002")

	var captured runtimehost.Config
	process := &fakeProcess{}
	command := newCommand(dependencies{
		version: "test-version",
		stdin:   bytes.NewReader(nil), stdout: io.Discard, stderr: io.Discard,
		userHomeDir: func() (string, error) { return "/home/test", nil },
		openRuntime: func(_ context.Context, config runtimehost.Config) (runtimeProcess, error) {
			captured = config
			return process, nil
		},
	})
	command.SetArgs([]string{
		"serve", "--config", configPath,
		"--listen", "127.0.0.1:18003",
		"--data-home", filepath.Join(root, "data"),
		"--workspace", "/workspace/explicit",
		"--cors-origin", "wails://localhost,http://wails.localhost",
	})
	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if captured.Listen != "127.0.0.1:18003" {
		t.Fatalf("listen = %q", captured.Listen)
	}
	if captured.ServerName != "from-file" || captured.ServerVersion != "test-version" {
		t.Fatalf("server identity = %q/%q", captured.ServerName, captured.ServerVersion)
	}
	if captured.DefaultWorkspace != "/workspace/explicit" || captured.UserHome != "/home/test" {
		t.Fatalf("host paths = %+v", captured)
	}
	if captured.DatabasePath != filepath.Join(root, "data", "runtime.sqlite") || captured.TokenPath != filepath.Join(root, "data", "local-token") {
		t.Fatalf("runtime paths = %+v", captured)
	}
	if len(captured.CORSOrigins) != 2 || captured.CORSOrigins[0] != "wails://localhost" {
		t.Fatalf("CORS origins = %v", captured.CORSOrigins)
	}
	if process.runs != 1 || process.closes != 1 {
		t.Fatalf("process lifecycle = runs:%d closes:%d", process.runs, process.closes)
	}
}

func TestEveryCommandGetsFreshViperAndFlagState(t *testing.T) {
	open := func(want string) func(context.Context, runtimehost.Config) (runtimeProcess, error) {
		return func(_ context.Context, config runtimehost.Config) (runtimeProcess, error) {
			if config.Listen != want {
				t.Fatalf("listen = %q, want %q", config.Listen, want)
			}
			return &fakeProcess{}, nil
		}
	}
	base := dependencies{
		version: "test", stdin: bytes.NewReader(nil), stdout: io.Discard, stderr: io.Discard,
		userHomeDir: func() (string, error) { return "/home/test", nil },
	}
	first := base
	first.openRuntime = open("127.0.0.1:19001")
	firstCommand := newCommand(first)
	firstCommand.SetArgs([]string{"serve", "--listen", "127.0.0.1:19001"})
	if err := firstCommand.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("first ExecuteContext() error = %v", err)
	}
	second := base
	second.openRuntime = open(defaultListen)
	secondCommand := newCommand(second)
	secondCommand.SetArgs([]string{"serve"})
	if err := secondCommand.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("second ExecuteContext() error = %v", err)
	}
}

type fakeProcess struct {
	runs   int
	closes int
}

func (process *fakeProcess) Run(context.Context) error {
	process.runs++
	return nil
}

func (process *fakeProcess) Close(context.Context) error {
	process.closes++
	return nil
}

func (process *fakeProcess) BaseURL() string { return "http://127.0.0.1:12345" }
