package bash

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func skipWithoutShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("no /bin/sh available")
	}
}

func TestLocalExecutor_Run_HappyPath(t *testing.T) {
	skipWithoutShell(t)
	out, err := NewLocalExecutor().Run(t.Context(), RunInput{Cmd: "echo hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(string(out.Stdout)); got != "hello" {
		t.Errorf("Stdout = %q, want %q", got, "hello")
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
	if out.Killed {
		t.Errorf("Killed = true, want false")
	}
}

func TestLocalExecutor_Run_NonZeroExit(t *testing.T) {
	skipWithoutShell(t)
	out, err := NewLocalExecutor().Run(t.Context(), RunInput{Cmd: "exit 7"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", out.ExitCode)
	}
}

func TestLocalExecutor_Run_StderrCaptured(t *testing.T) {
	skipWithoutShell(t)
	out, err := NewLocalExecutor().Run(t.Context(), RunInput{Cmd: "echo oops 1>&2"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(string(out.Stderr)); got != "oops" {
		t.Errorf("Stderr = %q, want %q", got, "oops")
	}
}

func TestLocalExecutor_Run_Timeout(t *testing.T) {
	skipWithoutShell(t)
	start := time.Now()
	out, err := NewLocalExecutor().Run(t.Context(), RunInput{
		Cmd:     "sleep 5",
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Killed {
		t.Errorf("Killed = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("elapsed %v > 2s — timeout didn't kick in", elapsed)
	}
}

func TestLocalExecutor_Run_EmptyCommand(t *testing.T) {
	_, err := NewLocalExecutor().Run(t.Context(), RunInput{Cmd: ""})
	if !errors.Is(err, ErrEmptyCommand) {
		t.Errorf("Run with empty Cmd: err = %v, want ErrEmptyCommand", err)
	}
}

func TestLocalExecutor_OutputCap(t *testing.T) {
	skipWithoutShell(t)
	exec := NewLocalExecutor()
	exec.MaxOutputBytes = 100
	out, err := exec.Run(t.Context(), RunInput{
		Cmd: `for i in $(seq 1 1000); do echo "line $i"; done`,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out.Stdout) > 200 {
		t.Errorf("Stdout length %d, expected ~100 + small marker", len(out.Stdout))
	}
	if !bytes.Contains(out.Stdout, []byte("truncated")) {
		t.Errorf("Stdout missing truncation marker: %q", out.Stdout)
	}
}

func TestTool_Definition(t *testing.T) {
	def := NewTool(nil).Definition()
	if def.Name != "bash" {
		t.Errorf("Name = %q, want %q", def.Name, "bash")
	}
	if def.InputSchema == "" {
		t.Error("InputSchema is empty")
	}
	if !strings.Contains(def.Description, "bash") {
		t.Errorf("Description missing 'bash': %q", def.Description)
	}
}

func TestTool_Call_HappyPath(t *testing.T) {
	skipWithoutShell(t)
	tool := NewTool(nil)
	result, err := tool.Call(t.Context(), `{"command":"echo hi"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp Response
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("Unmarshal response: %v\nbody=%s", err, result)
	}
	if !strings.Contains(resp.Stdout, "hi") {
		t.Errorf("Response.Stdout = %q, want substring %q", resp.Stdout, "hi")
	}
	if resp.ExitCode != 0 {
		t.Errorf("Response.ExitCode = %d, want 0", resp.ExitCode)
	}
}

func TestTool_Call_BadJSON(t *testing.T) {
	if _, err := NewTool(nil).Call(t.Context(), `{bad json}`); err == nil {
		t.Fatal("Call with bad JSON: want error")
	}
}

func TestTool_Call_NilExecutorDefaultsToLocal(t *testing.T) {
	skipWithoutShell(t)
	tool := NewTool(nil) // must not panic; should pick up LocalExecutor
	if _, err := tool.Call(t.Context(), `{"command":"true"}`); err != nil {
		t.Fatalf("Call with nil-executor default: %v", err)
	}
}

func TestBoundedBuffer_Truncates(t *testing.T) {
	b := newBoundedBuffer(10)
	payload := []byte("hello world this is more than 10 bytes")
	n, err := b.Write(payload)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(payload) {
		t.Errorf("Write returned %d, want %d (must pretend full write to keep pipe alive)", n, len(payload))
	}
	out := b.finalize()
	if !bytes.Contains(out, []byte("truncated")) {
		t.Errorf("finalize() missing marker: %q", out)
	}
	if b.dropped == 0 {
		t.Error("dropped = 0, want > 0")
	}
}

func TestBoundedBuffer_NoTruncation(t *testing.T) {
	b := newBoundedBuffer(100)
	if _, err := b.Write([]byte("short")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := b.finalize()
	if string(out) != "short" {
		t.Errorf("finalize() = %q, want %q", out, "short")
	}
	if bytes.Contains(out, []byte("truncated")) {
		t.Error("finalize() unexpectedly contains truncation marker")
	}
}
