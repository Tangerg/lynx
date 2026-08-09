package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ptytest"
)

func TestInteractiveBinaryReturnsTheTerminalIntact(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := filepath.Join(t.TempDir(), "lyra")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lyra: %v\n%s", err, output)
	}

	session, err := ptytest.StartWith(t.Context(), ptytest.Options{
		Size: ptytest.Size{Cols: 80, Rows: 24},
		Env: append(os.Environ(),
			"TERM=xterm-256color", "COLORTERM=truecolor",
			"HOME="+t.TempDir(), "USERPROFILE="+t.TempDir(),
		),
	}, binary)
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	const settle = 30 * time.Second
	if err := session.Transcript().WaitWithin(settle, "Ask lyra"); err != nil {
		t.Fatal(err)
	}
	if err := session.Type("\x03"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()
	if err := session.Wait(ctx); err != nil {
		t.Fatalf("lyra did not exit cleanly: %v", err)
	}

	transcript := session.Transcript().Bytes()
	ptytest.RequireNotContains(t, transcript, "\x1b[?1049h")
	ptytest.RequireContains(t, transcript, "\x1b[?25h")
	ptytest.RequireSymmetricModes(t, transcript,
		ptytest.Mode{Name: "bracketed paste", On: "\x1b[?2004h", Off: "\x1b[?2004l"},
	)
}
