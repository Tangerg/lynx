package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	for _, test := range []struct {
		name         string
		args         []string
		environment  map[string]string
		mouse        bool
		keyboardMode bool
	}{
		{
			name: "xterm truecolor", environment: map[string]string{
				"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
			},
			mouse: true, keyboardMode: true,
		},
		{
			name: "screen ascii without mouse", args: []string{"--mouse=false", "--notifications=false"},
			environment: map[string]string{
				"TERM": "screen-256color", "COLORTERM": "", "LANG": "C", "LC_ALL": "C",
			},
			mouse: false, keyboardMode: true,
		},
		{
			name: "vscode wsl keyboard guard", environment: map[string]string{
				"TERM": "xterm-256color", "TERM_PROGRAM": "vscode", "WSL_INTEROP": "/run/WSL/1_interop",
			},
			mouse: true, keyboardMode: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, err := ptytest.StartWith(t.Context(), ptytest.Options{
				Size: ptytest.Size{Cols: 80, Rows: 24},
				Env:  terminalTestEnvironment(t, test.environment),
			}, binary, test.args...)
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
			if err := session.Type("\x11"); err != nil {
				t.Fatal(err)
			}
			if err := session.Transcript().WaitWithin(settle, "press ctrl+q again to quit"); err != nil {
				t.Fatal(err)
			}
			if err := session.Type("\x11"); err != nil {
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
				ptytest.Mode{Name: "focus", On: "\x1b[?1004h", Off: "\x1b[?1004l"},
				ptytest.Mode{Name: "bracketed paste", On: "\x1b[?2004h", Off: "\x1b[?2004l"},
			)
			assertOptionalTerminalMode(t, transcript, test.mouse, ptytest.Mode{
				Name: "mouse", On: "\x1b[?1003h\x1b[?1006h", Off: "\x1b[?1006l\x1b[?1003l",
			})
			assertOptionalTerminalMode(t, transcript, test.keyboardMode, ptytest.Mode{
				Name: "keyboard", On: "\x1b[>5u", Off: "\x1b[<u",
			})
		})
	}
}

func terminalTestEnvironment(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	values := map[string]string{
		"COLORTERM": "", "LANG": "", "LC_ALL": "", "TERM_PROGRAM": "",
		"VSCODE_INJECTION": "", "WSL_INTEROP": "", "WSL_DISTRO_NAME": "",
	}
	for name, value := range overrides {
		values[name] = value
	}
	values["HOME"], values["USERPROFILE"] = t.TempDir(), t.TempDir()
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; !replaced {
			environment = append(environment, entry)
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func assertOptionalTerminalMode(t *testing.T, transcript []byte, enabled bool, mode ptytest.Mode) {
	t.Helper()
	if enabled {
		ptytest.RequireSymmetricModes(t, transcript, mode)
		return
	}
	ptytest.RequireNotContains(t, transcript, mode.On)
	ptytest.RequireNotContains(t, transcript, mode.Off)
}
