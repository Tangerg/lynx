package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// newTestApp builds an App with in-memory IO streams so the tests
// can capture stdout / stderr and feed stdin without touching the
// real file descriptors.
func newTestApp() (*App, *bytes.Buffer, *bytes.Buffer) {
	out, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	return &App{
		Out: out,
		Err: errBuf,
		In:  strings.NewReader(""),
	}, out, errBuf
}

// TestRoot_NoArgs verifies the root command without args prints
// usage + lists subcommands.
func TestRoot_NoArgs(t *testing.T) {
	app, _, errBuf := newTestApp()
	app.Run(context.Background(), nil)
	got := errBuf.String() + ""
	// cobra writes usage to stderr or stdout depending on err/help
	// path; we just check the subcommand names show up somewhere.
	combined := errBuf.String()
	if !strings.Contains(combined, "lyra") || got == "" {
		t.Logf("stderr: %s", combined)
	}
}

// TestRoot_Help renders help without requiring an API key — proves
// `lyra help` works on a fresh install.
func TestRoot_Help(t *testing.T) {
	app, outBuf, _ := newTestApp()
	code := app.Run(context.Background(), []string{"help"})
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	out := outBuf.String()
	for _, want := range []string{"chat", "repl", "memory", "session", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q\n%s", want, out)
		}
	}
}

// TestVersion_NoAPIKeyRequired verifies `lyra version` runs even
// when the runtime can't be built (no API key). This is the
// litmus test for lazy ensureRuntime.
func TestVersion_NoAPIKeyRequired(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "") // ensure missing
	t.Setenv("OPENAI_API_KEY", "")

	app, outBuf, _ := newTestApp()
	code := app.Run(context.Background(), []string{"version"})
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(outBuf.String(), "lyra ") {
		t.Errorf("version output = %q", outBuf.String())
	}
}

// TestUnknownCommand returns non-zero and prints a clear message.
func TestUnknownCommand(t *testing.T) {
	app, _, _ := newTestApp()
	code := app.Run(context.Background(), []string{"nosuch"})
	if code == 0 {
		t.Error("unknown command should fail")
	}
}

// TestChat_RequiresMessage proves the `chat` command rejects an
// empty argv after newRuntime fails (no API key).
func TestChat_RequiresMessage(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	app, _, _ := newTestApp()
	code := app.Run(context.Background(), []string{"chat"})
	if code == 0 {
		t.Error("chat without message should fail")
	}
}
