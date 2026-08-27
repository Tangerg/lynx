//go:build unix

package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	stdioProcessRoleEnv = "SCOPE_MCP_STDIO_PROCESS_ROLE"
	stdioDescendantPID  = "SCOPE_MCP_STDIO_DESCENDANT_PID"
)

func TestMain(m *testing.M) {
	switch os.Getenv(stdioProcessRoleEnv) {
	case "server":
		runStdioProcessServer()
		os.Exit(0)
	case "descendant":
		runStdioProcessDescendant()
		os.Exit(0)
	default:
		os.Exit(m.Run())
	}
}

func TestStdioSessionCleanupKillsDescendants(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "process-owner-test", Version: "v1"}, nil)
	session, cleanup, err := dial(t.Context(), t.Context(), client, ServerConfig{
		Name:      "process-owner-test",
		Transport: TransportStdio,
		Command:   os.Args[0],
		Env: withStdioProcessEnv(os.Environ(), map[string]string{
			stdioProcessRoleEnv: "server",
			stdioDescendantPID:  pidFile,
		}),
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial helper server: %v", err)
	}
	defer func() {
		_ = session.Close()
		_ = cleanup()
	}()

	pid := waitForStdioDescendantPID(t, pidFile)
	t.Cleanup(func() {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Errorf("clean leaked descendant %d: %v", pid, err)
		}
	})

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup session process: %v", err)
	}
	if waitForStdioProcessExit(pid, 2*time.Second) {
		return
	}
	t.Fatalf("stdio session descendant %d survived session cleanup", pid)
}

func runStdioProcessServer() {
	descendant := exec.Command(os.Args[0]) //nolint:noctx // the test process must outlive its parent
	descendant.Env = withStdioProcessEnv(os.Environ(), map[string]string{
		stdioProcessRoleEnv: "descendant",
	})
	if err := descendant.Start(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "start descendant: %v\n", err)
		os.Exit(2)
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "process-owner-server", Version: "v1"}, nil)
	if err := server.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "run MCP server: %v\n", err)
		os.Exit(2)
	}
}

func runStdioProcessDescendant() {
	pidFile := os.Getenv(stdioDescendantPID)
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(2)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func waitForStdioDescendantPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil {
				t.Fatalf("parse descendant PID: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read descendant PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stdio helper did not publish its descendant PID")
	return 0
}

func waitForStdioProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func withStdioProcessEnv(base []string, replacements map[string]string) []string {
	env := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replace := replacements[key]; !replace {
			env = append(env, entry)
		}
	}
	for key, value := range replacements {
		env = append(env, key+"="+value)
	}
	return env
}
