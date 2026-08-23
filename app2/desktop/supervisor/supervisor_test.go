package supervisor

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
)

func TestSupervisorRestartsKilledRuntimeAndReleasesEveryGeneration(t *testing.T) {
	runtimeBinary := buildRuntime(t)
	dataHome := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dataHome, 0o700); err != nil {
		t.Fatalf("Mkdir(data) error = %v", err)
	}
	supervisor, err := New(Config{
		RuntimeBinary:     runtimeBinary,
		DataHome:          dataHome,
		DefaultWorkspace:  "/workspace",
		UserHome:          "/home/test",
		CORSOrigins:       httptransport.DefaultCORSOrigins(),
		StartupTimeout:    5 * time.Second,
		ProbeTimeout:      time.Second,
		ShutdownTimeout:   3 * time.Second,
		RestartBackoff:    10 * time.Millisecond,
		MaxRestartBackoff: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	first, err := supervisor.Start(t.Context())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if first.Endpoint == "" || first.LocalToken == "" || first.InstanceID == "" || first.Generation != 1 {
		t.Fatalf("first connection = %+v", first.redacted())
	}
	firstRoot := supervisor.generationRoot()
	if err := first.process.Kill(); err != nil {
		t.Fatalf("kill first Runtime: %v", err)
	}

	second := awaitSuccessor(t, supervisor, first.InstanceID)
	if second.Generation != 2 || second.process.Pid == first.process.Pid {
		t.Fatalf("successor = %+v, predecessor PID = %d", second.redacted(), first.process.Pid)
	}
	if _, err := os.Stat(firstRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("predecessor generation root remains: %v", err)
	}
	secondRoot := supervisor.generationRoot()
	secondAddress := endpointAddress(t, second.Endpoint)
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := os.Stat(secondRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successor generation root remains: %v", err)
	}
	if connection, err := net.DialTimeout("tcp", secondAddress, 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("successor listener remains after Close")
	}
	if err := second.process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("successor process remains after Close")
	}
}

func TestSupervisorFailsClosedWhenChildCannotPublish(t *testing.T) {
	supervisor, err := New(Config{
		RuntimeBinary:    "/usr/bin/false",
		DataHome:         privateDataHome(t),
		DefaultWorkspace: "/workspace", UserHome: "/home/test",
		StartupTimeout: time.Second, ProbeTimeout: 100 * time.Millisecond,
		ShutdownTimeout: time.Second, MaxStartupAttempts: 2,
		RestartBackoff: time.Millisecond, MaxRestartBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := supervisor.Start(t.Context()); err == nil {
		t.Fatal("Start() accepted a child that never published a descriptor")
	}
	if _, err := supervisor.Connection(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connection() error = %v, want ErrUnavailable", err)
	}
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if root := supervisor.generationRoot(); root != "" {
		t.Fatalf("failed generation root remains: %q", root)
	}
}

func buildRuntime(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "lyra-runtime")
	command := exec.Command("go", "build", "-o", binary, "./cmd/lyra-runtime")
	command.Dir = "../../runtime"
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build app2 Runtime: %v\n%s", err, output)
	}
	return binary
}

func awaitSuccessor(t *testing.T, supervisor *Supervisor, predecessor string) Connection {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		connection, err := supervisor.Connection()
		if err == nil && connection.InstanceID != predecessor {
			return connection
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("successor did not become ready; last error = %v", err)
		}
	}
}

func privateDataHome(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	return path
}

func endpointAddress(t *testing.T, endpoint string) string {
	t.Helper()
	const prefix = "http://"
	if len(endpoint) <= len(prefix) || endpoint[:len(prefix)] != prefix {
		t.Fatalf("invalid endpoint %q", endpoint)
	}
	return endpoint[len(prefix):]
}
