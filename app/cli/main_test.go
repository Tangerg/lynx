package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ptytest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/backend"
	"github.com/Tangerg/lynx/app/cli/internal/terminal"
)

type terminalModeCase struct {
	name         string
	args         []string
	environment  map[string]string
	mouse        bool
	keyboardMode bool
}

func TestInteractiveBinaryReturnsTheTerminalIntact(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)

	for _, test := range []terminalModeCase{
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
		t.Run(test.name, func(t *testing.T) { requireTerminalModeLifecycle(t, binary, test) })
	}
}

func requireTerminalModeLifecycle(t *testing.T, binary string, test terminalModeCase) {
	t.Helper()
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: ptytest.Size{Cols: 80, Rows: 24}, Env: terminalTestEnvironment(t, test.environment),
	}, binary, test.args...)
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitForVisibleTerminalText(t, session, ptytest.Size{Cols: 80, Rows: 24}, "Ask lyra")
	if _, err := io.WriteString(session, "\x11"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, ptytest.Size{Cols: 80, Rows: 24}, "repeat ctrl+q or ctrl+d to quit")
	if _, err := io.WriteString(session, "\x11"); err != nil {
		t.Fatal(err)
	}
	waitForInteractiveExit(t, session)

	transcript := session.Transcript().Bytes()
	ptytest.RequireContains(t, transcript, "\x1b[?25h")
	ptytest.RequireSymmetricModes(t, transcript,
		ptytest.Mode{Name: "alternate screen", On: "\x1b[?1049h", Off: "\x1b[?1049l"},
		ptytest.Mode{Name: "focus", On: "\x1b[?1004h", Off: "\x1b[?1004l"},
		ptytest.Mode{Name: "bracketed paste", On: "\x1b[?2004h", Off: "\x1b[?2004l"},
	)
	assertOptionalTerminalMode(t, transcript, test.mouse, ptytest.Mode{
		Name: "mouse", On: "\x1b[?1003h\x1b[?1006h", Off: "\x1b[?1006l\x1b[?1003l",
	})
	assertOptionalTerminalMode(t, transcript, test.keyboardMode, ptytest.Mode{
		Name: "keyboard", On: "\x1b[>5u", Off: "\x1b[<u",
	})
}

type submittedInputCase struct {
	name  string
	input string
	want  []string
}

func TestInteractiveBinaryPreservesSubmittedInputSequences(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)
	size := ptytest.Size{Cols: 100, Rows: 30}

	for _, test := range []submittedInputCase{
		{
			name:  "fast utf8 input followed by enter",
			input: "contract-tail-你好终\r",
			want:  []string{"contract-tail-你好终"},
		},
		{
			name:  "long input burst preserves its unicode tail",
			input: strings.Repeat("rapid-input-", 64) + "最后字符终\r",
			want:  []string{"最后字符终"},
		},
		{
			name:  "kitty associated text preserves one composed input event",
			input: "associated-\x1b[120;1;20320:22909u-tail\r",
			want:  []string{"associated-你好-tail"},
		},
		{
			name:  "combining and zwj graphemes survive at the submission tail",
			input: "grapheme-tail-e\u0301-👩\u200d💻\r",
			want:  []string{"grapheme-tail-e\u0301-👩\u200d💻"},
		},
		{
			name:  "kitty shift enter inserts a newline",
			input: "kitty-first\x1b[13;2u第二行-kitty-tail\r",
			want:  []string{"kitty-first", "第二行-kitty-tail"},
		},
		{
			name:  "legacy alt enter inserts a newline",
			input: "legacy-first\x1b\r第二行-legacy-tail\r",
			want:  []string{"legacy-first", "第二行-legacy-tail"},
		},
		{
			name:  "bracketed paste and trailing enter stay separate",
			input: "\x1b[200~pasted-first\npasted-tail-终\x1b[201~\r",
			want:  []string{"pasted-first", "pasted-tail-终"},
		},
		{
			name:  "paste terminator trailing character and enter stay ordered",
			input: "\x1b[200~paste-body\x1b[201~末\r",
			want:  []string{"paste-body末"},
		},
	} {
		t.Run(test.name, func(t *testing.T) { requireSubmittedInputSequence(t, binary, size, test) })
	}
}

func requireSubmittedInputSequence(t *testing.T, binary string, size ptytest.Size, test submittedInputCase) {
	t.Helper()
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: size,
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
		}),
	}, binary, "--mouse=false", "--notifications=false")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	waitForVisibleTerminalText(t, session, size, "Ask lyra")
	if written, err := io.WriteString(session, test.input); err != nil || written != len(test.input) {
		t.Fatalf("write input = (%d, %v), want %d bytes", written, err, len(test.input))
	}
	// This item appears only after the runtime receives the submission, so it
	// proves Enter was decoded separately from the preceding input bytes.
	waitForVisibleTerminalText(t, session, size, "Reproduce the flake")
	quitInteractiveSession(t, session)
	screen, err := session.Transcript().Screen(size)
	if err != nil {
		t.Fatalf("decode final screen: %v", err)
	}
	visible := strings.Join(screen.Rows(), "\n")
	for _, token := range test.want {
		if !strings.Contains(visible, token) {
			t.Errorf("submitted screen does not contain %q:\n%s", token, visible)
		}
	}
}

func TestInteractiveBinaryConsumesTheStartupBrandOnce(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)
	size := ptytest.Size{Cols: 96, Rows: 24}
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: size,
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
		}),
	}, binary, "--mouse=false", "--notifications=false")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	waitForVisibleTerminalText(t, session, size, "Lyra CLI", "Ask lyra")
	narrowSize := ptytest.Size{Cols: 36, Rows: 18}
	if err := session.Resize(narrowSize); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, narrowSize, "LYRA", "Ask lyra")
	if err := session.Resize(size); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "Lyra CLI", "Ask lyra")
	if _, err := io.WriteString(session, "startup-banner-contract\r"); err != nil {
		t.Fatal(err)
	}
	afterPrompt := waitForVisibleTerminalText(t, session, size, "startup-banner-contract", "Reproduce the flake")
	if visible := strings.Join(afterPrompt.Rows(), "\n"); strings.Contains(visible, "Lyra CLI") {
		t.Fatalf("conversation retained the startup brand:\n%s", visible)
	}

	waitForVisibleTerminalText(t, session, size, "Tool approval")
	if _, err := io.WriteString(session, "\x03"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "$0.0291", "complete")
	if _, err := io.WriteString(session, "/clear\r"); err != nil {
		t.Fatal(err)
	}
	cleared := waitForVisibleTerminalText(t, session, size, "cleared", "Ask lyra")
	if visible := strings.Join(cleared.Rows(), "\n"); strings.Contains(visible, "Lyra CLI") || strings.Contains(visible, "LYRA") {
		t.Fatalf("clear restored the consumed startup brand:\n%s", visible)
	}
	quitInteractiveSession(t, session)
}

func TestInteractiveBinarySurvivesResizeAndApprovalRoundTrip(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: ptytest.Size{Cols: 96, Rows: 24},
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
		}),
	}, binary, "--mouse=false", "--notifications=false")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	waitForVisibleTerminalText(t, session, ptytest.Size{Cols: 96, Rows: 24}, "Ask lyra")
	if _, err := io.WriteString(session, "resize-hitl-contract\r"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, ptytest.Size{Cols: 96, Rows: 24}, "Tool approval")
	narrowAfter := len(session.Transcript().Bytes())
	narrowSize := ptytest.Size{Cols: 70, Rows: 18}
	if err := session.Resize(narrowSize); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(session, "\tNARROW_APPROVAL_MARKER"); err != nil {
		t.Fatal(err)
	}
	narrow := waitForTerminalScreen(t, session, narrowSize, narrowAfter, "Tool approval", "NARROW_APPROVAL_MARKER")
	requireFullViewportDialog(t, narrow, "Tool approval")
	wideAfter := len(session.Transcript().Bytes())
	wideSize := ptytest.Size{Cols: 128, Rows: 36}
	if err := session.Resize(wideSize); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(session, "_WIDE_MARKER"); err != nil {
		t.Fatal(err)
	}
	waitForTerminalScreen(t, session, wideSize, wideAfter, "Tool approval", "NARROW_APPROVAL_MARKER_WIDE_MARKER")
	if _, err := io.WriteString(session, "\r"); err != nil {
		t.Fatal(err)
	}
	waitForTerminalScreen(t, session, wideSize, wideAfter, "Ran the test 50 times", "$0.0412", "complete")
	quitInteractiveSession(t, session)
}

const mixedInteractionPTYScenario = "mixed-interaction-contract"

func TestInteractiveTerminalCompletesMixedInteractionReviewThroughPTY(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	size := ptytest.Size{Cols: 96, Rows: 28}
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: size,
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
			"LYRA_PTY_TEST_SCENARIO": mixedInteractionPTYScenario,
		}),
	}, os.Args[0], "-test.run=^TestMixedInteractionPTYRuntime$")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	waitForVisibleTerminalText(t, session, size, "Ask lyra")
	writeTerminalInput(t, session, "exercise mixed interactions\r")
	waitForVisibleTerminalText(t, session, size, "Run contract checks", "Allow once")
	writeTerminalInput(t, session, strings.Repeat("\x1b[B", 8)+"\r")
	waitForVisibleTerminalText(t, session, size, "Edit tool arguments", `"command": "go test ./..."`)
	writeTerminalInput(t, session, "\x1ba"+`{"command":"go test ./...","count":2}`+"\x13")
	waitForVisibleTerminalText(t, session, size, "Edited arguments · one-shot", `"count": 2`)
	writeTerminalInput(t, session, "\x1b[B\x1b[B\r")
	waitForVisibleTerminalText(t, session, size, "Describe intent")
	writeTerminalInput(t, session, "ship safely\r")
	waitForVisibleTerminalText(t, session, size, "Choose platform", "Linux")
	writeTerminalInput(t, session, "\x1b[B\r")
	waitForVisibleTerminalText(t, session, size, "Choose checks", "Unit", "Integration")
	writeTerminalInput(t, session, " \r")
	waitForVisibleTerminalText(t, session, size, "Review interactions")

	minimal := ptytest.Size{Cols: 1, Rows: 1}
	if err := session.Resize(minimal); err != nil {
		t.Fatal(err)
	}
	if err := session.Resize(size); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "Review interactions", "Submit all decisions")
	writeTerminalInput(t, session, "\x1b[B\r")
	waitForVisibleTerminalText(t, session, size, "Choose checks", "Unit", "Integration")
	writeTerminalInput(t, session, "\x1b[B \r")
	waitForVisibleTerminalText(t, session, size, "Review interactions")
	writeTerminalInput(t, session, "\r")
	waitForVisibleTerminalText(t, session, size, "PTY HITL contract accepted", "complete")
	quitInteractiveSession(t, session)
}

// TestMixedInteractionPTYRuntime is launched as the child process above. The
// environment guard keeps the ordinary package test run non-interactive.
func TestMixedInteractionPTYRuntime(t *testing.T) {
	if os.Getenv("LYRA_PTY_TEST_SCENARIO") != mixedInteractionPTYScenario {
		return
	}
	backend := mock.New()
	backend.Instant = true
	backend.Script = func(string) mock.Script { return mixedInteractionPTYScript() }
	if err := terminal.Run(t.Context(), terminal.Config{Runtime: backend, Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
}

func mixedInteractionPTYScript() mock.Script {
	return mock.Script{
		Interactions: []agent.Interaction{
			agent.Approval{
				ItemID: "approval", Title: "Run contract checks", Rememberable: true,
				RuleHint: "shell:go test ./...",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning,
					ArgumentsJSON: []byte(`{"command":"go test ./...","count":1}`),
				},
			},
			agent.Question{
				ItemID: "intent", Title: "Describe intent",
				Fields: []agent.QuestionField{{Prompt: "Intent", Kind: agent.QuestionText}},
			},
			agent.Question{
				ItemID: "platform", Title: "Choose platform",
				Fields: []agent.QuestionField{{
					Prompt: "Platform", Kind: agent.QuestionSingle,
					Options: []agent.QuestionOption{{Label: "Linux"}, {Label: "Darwin"}},
				}},
			},
			agent.Question{
				ItemID: "checks", Title: "Choose checks",
				Fields: []agent.QuestionField{{
					Prompt: "Checks", Kind: agent.QuestionMulti,
					Options: []agent.QuestionOption{{Label: "Unit"}, {Label: "Integration"}},
				}},
			},
		},
		Continue: func(provided []agent.InterruptAnswer) []mock.Step {
			result := "PTY HITL contract rejected"
			if mixedInteractionPTYAnswersMatch(provided) {
				result = "PTY HITL contract accepted"
			}
			return []mock.Step{
				{Event: agent.BlockCompleted{Block: agent.Block{ID: "result", Kind: agent.BlockNotice, Text: result}}},
				{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}
		},
	}
}

func mixedInteractionPTYAnswersMatch(provided []agent.InterruptAnswer) bool {
	if len(provided) != 4 {
		return false
	}
	approval, ok := provided[0].Answer.(agent.ApprovalAnswer)
	if !ok || approval.Decision != agent.ApprovalApprove || approval.Remember != agent.RememberProject ||
		approval.ArgumentOverride == nil ||
		string(approval.ArgumentOverride.JSON()) != `{"command":"go test ./...","count":2}` {
		return false
	}
	intent, ok := provided[1].Answer.(agent.QuestionAnswer)
	if !ok || len(intent.Values) != 1 || !slices.Equal(intent.Values[0], []string{"ship safely"}) {
		return false
	}
	platform, ok := provided[2].Answer.(agent.QuestionAnswer)
	if !ok || len(platform.Values) != 1 || !slices.Equal(platform.Values[0], []string{"Darwin"}) {
		return false
	}
	checks, ok := provided[3].Answer.(agent.QuestionAnswer)
	return ok && len(checks.Values) == 1 && slices.Equal(checks.Values[0], []string{"Unit", "Integration"})
}

func writeTerminalInput(t *testing.T, session *ptytest.Session, value string) {
	t.Helper()
	if written, err := io.WriteString(session, value); err != nil || written != len(value) {
		t.Fatalf("write terminal input = (%d, %v), want %d bytes", written, err, len(value))
	}
}

func requireFullViewportDialog(t *testing.T, screen *ptytest.Screen, title string) {
	t.Helper()
	rows := screen.Rows()
	if len(rows) < 2 || !strings.HasPrefix(rows[0], "╭─"+title) || !strings.HasSuffix(rows[0], "╮") {
		t.Fatalf("dialog does not own its top viewport edge:\n%s", strings.Join(rows, "\n"))
	}
	for index, row := range rows[1 : len(rows)-1] {
		if !strings.HasPrefix(row, "│") || !strings.HasSuffix(row, "│") {
			t.Fatalf("dialog row %d leaked the covered interface:\n%s", index+1, strings.Join(rows, "\n"))
		}
	}
	if !strings.HasPrefix(rows[len(rows)-1], "╰") || !strings.HasSuffix(rows[len(rows)-1], "╯") {
		t.Fatalf("dialog does not own its bottom viewport edge:\n%s", strings.Join(rows, "\n"))
	}
}

func TestInteractiveBinaryExpandsCompletedToolFromTranscript(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)
	size := ptytest.Size{Cols: 120, Rows: 40}
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: size,
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
		}),
	}, binary, "--mouse=false", "--notifications=false")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	waitForVisibleTerminalText(t, session, size, "Ask lyra")
	if _, err := io.WriteString(session, "tool-expand-contract\r"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "Tool approval")
	if _, err := io.WriteString(session, "\r"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "Ran the test 50 times", "$0.0412")
	// The retained selection starts at the last assistant entry. Move to the
	// preceding completed tool and expand it from the keyboard.
	if _, err := io.WriteString(session, "\t\x1b[A\r"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "2.104s")
	quitInteractiveSession(t, session)

	screen, err := session.Transcript().Screen(size)
	if err != nil {
		t.Fatalf("decode final screen: %v", err)
	}
	visible := strings.Join(screen.Rows(), "\n")
	if !strings.Contains(visible, "2.104s") {
		t.Fatalf("expanded tool output is not visible:\n%s", visible)
	}
}

func TestInteractiveBinaryKeepsScrolledTranscriptStableWhileStreaming(t *testing.T) {
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	binary := buildTestBinary(t)
	size := ptytest.Size{Cols: 80, Rows: 22}
	session, err := ptytest.Start(t.Context(), ptytest.Config{
		Size: size,
		Env: terminalTestEnvironment(t, map[string]string{
			"TERM": "xterm-256color", "COLORTERM": "truecolor", "LANG": "en_US.UTF-8",
		}),
	}, binary, "--mouse=false", "--notifications=false")
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	waitForVisibleTerminalText(t, session, size, "Ask lyra")
	if _, err := io.WriteString(session, "stream-scroll-contract\r"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "The sleep is the bug")
	if _, err := io.WriteString(session, "\x1b[5~"); err != nil {
		t.Fatal(err)
	}
	// The modal is painted independently of the transcript viewport, so it proves
	// the stream reached its interrupt without requiring the newly appended tail
	// to become visible after bottom-following was suspended.
	waitForVisibleTerminalText(t, session, size, "Tool approval")
	if _, err := io.WriteString(session, "\x03"); err != nil {
		t.Fatal(err)
	}
	waitForVisibleTerminalText(t, session, size, "$0.0291")
	quitInteractiveSession(t, session)

	screen, err := session.Transcript().Screen(size)
	if err != nil {
		t.Fatalf("decode final screen: %v", err)
	}
	visible := strings.Join(screen.Rows(), "\n")
	if !strings.Contains(visible, "go test ./internal/store") && !strings.Contains(visible, "roughly one run in five") {
		t.Fatalf("streaming moved a user-scrolled transcript back to the bottom:\n%s", visible)
	}
}

func waitForVisibleTerminalText(t *testing.T, session *ptytest.Session, size ptytest.Size, tokens ...string) *ptytest.Screen {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastVisible string
	var lastErr error
	for {
		screen, err := session.Transcript().Screen(size)
		lastErr = err
		if err == nil {
			visible := strings.Join(screen.Rows(), "\n")
			lastVisible = visible
			if containsEvery(visible, tokens) {
				return screen
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("terminal screen never showed %q: %v (decode: %v)\n%s", tokens, context.Cause(ctx), lastErr, lastVisible)
		}
	}
}

func waitForTerminalScreen(t *testing.T, session *ptytest.Session, size ptytest.Size, after int, tokens ...string) *ptytest.Screen {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastVisible string
	var lastErr error
	for {
		transcript := session.Transcript()
		written := transcript.Bytes()
		if len(written) > after {
			screen, err := ptytest.NewScreen(size)
			if err == nil {
				err = screen.Apply(written[after:])
			}
			if err == nil {
				err = screen.Flush()
			}
			lastErr = err
			if err == nil {
				visible := strings.Join(screen.Rows(), "\n")
				lastVisible = visible
				if containsEvery(visible, tokens) {
					return screen
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("terminal screen never showed %q: %v (decode: %v)\n%s", tokens, context.Cause(ctx), lastErr, lastVisible)
		}
	}
}

func containsEvery(value string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(value, token) {
			return false
		}
	}
	return true
}

func quitInteractiveSession(t *testing.T, session *ptytest.Session) {
	t.Helper()
	if _, err := io.WriteString(session, "\x11\x11"); err != nil {
		t.Fatal(err)
	}
	waitForInteractiveExit(t, session)
}

func waitForInteractiveExit(t *testing.T, session *ptytest.Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := session.Wait(ctx); err != nil {
		t.Fatalf("lyra did not exit cleanly: %v", err)
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "lyra")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lyra: %v\n%s", err, output)
	}
	return binary
}

func TestHeadlessBinaryReturnsConventionalSignalExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix signal exit semantics")
	}
	binary := buildTestBinary(t)
	for _, test := range []struct {
		name   string
		signal os.Signal
		want   int
	}{
		{name: "SIGINT", signal: os.Interrupt, want: 130},
		{name: "SIGTERM", signal: syscall.SIGTERM, want: 143},
	} {
		t.Run(test.name, func(t *testing.T) { requireHeadlessSignalExit(t, binary, test.signal, test.want) })
	}
}

func requireHeadlessSignalExit(t *testing.T, binary string, signal os.Signal, want int) {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, "run", "--json", "--approve-all", "wait for signal")
	command.Env = terminalTestEnvironment(t, nil)
	command.Stdout = io.Discard
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waiting := false
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			if !waiting {
				_ = command.Wait()
			}
		}
	})

	var diagnostics bytes.Buffer
	ready, scanned := scanRuntimeNotice(stderr, &diagnostics)
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for headless run to start")
	}
	if err := command.Process.Signal(signal); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	waiting = true
	go func() { waited <- command.Wait() }()
	select {
	case err := <-waited:
		waiting = false
		if scanErr := <-scanned; scanErr != nil {
			t.Fatalf("read stderr: %v", scanErr)
		}
		exit, ok := errors.AsType[*exec.ExitError](err)
		if !ok || exit.ExitCode() != want {
			t.Fatalf("signal exit = %v, want code %d\nstderr:\n%s", err, want, diagnostics.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("headless run did not stop after signal")
	}
}

func scanRuntimeNotice(stderr io.Reader, diagnostics *bytes.Buffer) (<-chan error, <-chan error) {
	ready := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		notified := false
		for scanner.Scan() {
			line := scanner.Text()
			diagnostics.WriteString(line)
			diagnostics.WriteByte('\n')
			if !notified && strings.Contains(line, mockNoticeForTest) {
				ready <- nil
				notified = true
			}
		}
		if err := scanner.Err(); err != nil {
			if !notified {
				ready <- err
			}
			done <- err
			return
		}
		if !notified {
			ready <- errors.New("process exited before opening the mock runtime")
		}
		done <- nil
	}()
	return ready, done
}

const mockNoticeForTest = "scripted mock runtime"

func TestRuntimeOwnerSelectionRejectsAmbiguousConfiguration(t *testing.T) {
	t.Setenv("LYRA_RUNTIME", "typo")
	if _, _, err := newRuntimeOwner(); err == nil {
		t.Fatal("newRuntimeOwner accepted an unknown mode")
	}

	t.Setenv("LYRA_RUNTIME", "embedded")
	t.Setenv("LYRA_HOME", "relative")
	if _, _, err := newRuntimeOwner(); err == nil {
		t.Fatal("newRuntimeOwner accepted a relative LYRA_HOME")
	}

	t.Setenv("LYRA_RUNTIME", "mock")
	owner, notice, err := newRuntimeOwner()
	if err != nil || owner == nil || !strings.Contains(notice, mockNoticeForTest) {
		t.Fatalf("mock owner = (%T, %q, %v)", owner, notice, err)
	}
}

type testExitError struct{ code int }

func (e testExitError) Error() string { return "coded" }
func (e testExitError) ExitCode() int { return e.code }

type scriptedRuntimeOwner struct {
	closeErrors []error
	closes      int
}

func (*scriptedRuntimeOwner) Runtime(context.Context) (backend.Services, error) {
	return backend.Services{}, nil
}

func (owner *scriptedRuntimeOwner) Close() error {
	index := owner.closes
	owner.closes++
	if index >= len(owner.closeErrors) {
		return nil
	}
	return owner.closeErrors[index]
}

func TestRuntimeOwnerCloseResumesIncompleteTeardown(t *testing.T) {
	t.Parallel()
	transient := errors.New("teardown still draining")
	owner := &scriptedRuntimeOwner{closeErrors: []error{transient, transient, nil}}
	if err := closeRuntimeOwner(owner); err != nil {
		t.Fatalf("closeRuntimeOwner: %v", err)
	}
	if owner.closes != 3 {
		t.Fatalf("close attempts = %d, want 3", owner.closes)
	}
}

func TestRuntimeOwnerCloseBoundsPermanentFailure(t *testing.T) {
	t.Parallel()
	want := errors.New("permanent teardown failure")
	owner := &scriptedRuntimeOwner{closeErrors: []error{want, want, want, nil}}
	err := closeRuntimeOwner(owner)
	if !errors.Is(err, want) || owner.closes != runtimeCloseAttempts {
		t.Fatalf("close result = (%v, %d attempts)", err, owner.closes)
	}
}

func TestExitCodePreservesSignalsAndClassifiesCancellation(t *testing.T) {
	if got := exitCode(testExitError{code: 143}); got != 143 {
		t.Fatalf("coded exit = %d, want 143", got)
	}
	if got := exitCode(context.Canceled); got != 130 {
		t.Fatalf("canceled exit = %d, want 130", got)
	}
	if got := exitCode(errors.New("ordinary")); got != 1 {
		t.Fatalf("ordinary exit = %d, want 1", got)
	}
}

func TestProcessSignalExitCodes(t *testing.T) {
	for _, test := range []struct {
		signal os.Signal
		want   int
	}{
		{signal: os.Interrupt, want: 130},
		{signal: syscall.SIGTERM, want: 143},
	} {
		if got := (processSignalError{signal: test.signal}).ExitCode(); got != test.want {
			t.Errorf("signal %s exit = %d, want %d", test.signal, got, test.want)
		}
	}
}

func terminalTestEnvironment(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	values := map[string]string{
		"COLORTERM": "", "LANG": "", "LC_ALL": "", "TERM_PROGRAM": "",
		"VSCODE_INJECTION": "", "WSL_INTEROP": "", "WSL_DISTRO_NAME": "",
		"LYRA_RUNTIME": "mock", "LYRA_RUNTIME_CONFIG_DIR": "",
	}
	maps.Copy(values, overrides)
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
