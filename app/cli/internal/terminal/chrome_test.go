package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestSessionHeaderUsesSpaceProgressively(t *testing.T) {
	header := newSessionHeader(kit.Dark(), kit.Unicode(), client.Session{
		Title:     "Architecture review",
		Workspace: "/workspace/lynx",
	})
	header.SetUsage(client.Usage{InputTokens: 1_234, OutputTokens: 56_789})

	if got := header.Measure(headerMinWidth - 1); got != 0 {
		t.Fatalf("narrow header height = %d, want 0", got)
	}
	if got := header.Measure(headerMinWidth); got != 2 {
		t.Fatalf("wide header height = %d, want 2", got)
	}

	wide := drawStatic(t, header, 72, 2)
	for _, want := range []string{"/workspace/lynx", "Architecture review", "↑1.2k", "↓56k"} {
		if !strings.Contains(wide, want) {
			t.Errorf("header does not contain %q:\n%s", want, wide)
		}
	}

	narrow := drawStatic(t, header, headerMinWidth-1, 2)
	if strings.TrimSpace(narrow) != "" {
		t.Fatalf("narrow header should yield its space, got:\n%s", narrow)
	}
}

func TestActivityViewCentersACompactWindowOnTheActiveStep(t *testing.T) {
	activity := newActivityView(kit.Dark(), kit.Unicode())
	activity.Set([]client.PlanItem{
		{Title: "Inspect references", Status: client.PlanDone},
		{Title: "Define shell", Status: client.PlanDone},
		{Title: "Build prompt", Status: client.PlanPending},
		{Title: "Refine tools", Status: client.PlanActive},
		{Title: "Add responsive tests", Status: client.PlanPending},
		{Title: "Run quality gates", Status: client.PlanPending},
	})

	got := drawStatic(t, activity, 44, activity.Measure(44))
	for _, want := range []string{"Plan", "step 4/6", "Build prompt", "Refine tools", "Add responsive tests"} {
		if !strings.Contains(got, want) {
			t.Errorf("activity does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Inspect references") || strings.Contains(got, "Run quality gates") {
		t.Fatalf("activity did not window its long plan:\n%s", got)
	}
	if got := activity.Measure(activityMinWidth - 1); got != 0 {
		t.Fatalf("narrow activity height = %d, want 0", got)
	}
}

func TestCompactTokensKeepsTheSignAndReadablePrecision(t *testing.T) {
	tests := map[int64]string{
		0:         "0",
		999:       "999",
		1_234:     "1.2k",
		-1_234:    "-1.2k",
		56_789:    "56k",
		2_500_000: "2.5m",
	}
	for tokens, want := range tests {
		if got := compactTokens(tokens); got != want {
			t.Errorf("compactTokens(%d) = %q, want %q", tokens, got, want)
		}
	}
}

func TestPromptMovesRunOptionsIntoTheFrameAndChangesContext(t *testing.T) {
	keys, err := configuredKeys(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	composer := kit.Composer{Theme: kit.Dark(), Prompt: "> "}
	prompt := newPromptView(kit.Dark(), kit.Unicode(), keys, &composer, settings.Default().RunOptions())
	prompt.Focus(true)

	idle := drawRoot(t, prompt, 120, prompt.Measure(120))
	for _, want := range []string{"runtime default · medium · build · ask", "enter", "shift+enter", "ctrl+p"} {
		if !strings.Contains(idle, want) {
			t.Errorf("idle prompt does not contain %q:\n%s", want, idle)
		}
	}

	prompt.SetBusy(true)
	busy := drawRoot(t, prompt, 120, prompt.Measure(120))
	for _, want := range []string{"enter", "queue follow up", "ctrl+c", "shift+enter", "ctrl+o"} {
		if !strings.Contains(busy, want) {
			t.Errorf("busy prompt does not contain %q:\n%s", want, busy)
		}
	}
	if strings.Contains(busy, "ctrl+r") {
		t.Fatalf("busy prompt retained an idle-only session hint:\n%s", busy)
	}
}

func TestShellRendersAtSupportedAndConstrainedTerminalSizes(t *testing.T) {
	keys, err := configuredKeys(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testConversationView(t)
	header := newSessionHeader(theme, glyphs, client.Session{Title: "New session", Workspace: "/workspace/lynx"})
	activity := newActivityView(theme, glyphs)
	activity.Set([]client.PlanItem{{Title: "Inspect", Status: client.PlanActive}})
	status := newStatusView(theme, glyphs, settings.Default().RunOptions())
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	prompt := newPromptView(theme, glyphs, keys, &composer, settings.Default().RunOptions())
	shell := newShellView(header, transcript, activity, newQueueView(theme, glyphs), status, prompt)
	shell.Focus(true)

	for _, size := range []struct{ width, height int }{{96, 28}, {44, 18}, {20, 8}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			got := drawRoot(t, shell, size.width, size.height)
			if strings.TrimSpace(got) == "" {
				t.Fatalf("%dx%d shell rendered nothing", size.width, size.height)
			}
		})
	}
}

func TestShellMovesFocusBetweenPromptAndTranscript(t *testing.T) {
	keys, err := configuredKeys(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testConversationView(t)
	appendTestTool(transcript, "focus", "detail")
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	prompt := newPromptView(theme, glyphs, keys, &composer, settings.Default().RunOptions())
	shell := newShellView(
		newSessionHeader(theme, glyphs, client.Session{}), transcript,
		newActivityView(theme, glyphs), newQueueView(theme, glyphs),
		newStatusView(theme, glyphs, settings.Default().RunOptions()), prompt,
	)
	prompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(prompt.SetTranscriptFocused)
	transcript.OnSelection(prompt.SetTranscriptSelection)
	shell.Focus(true)

	if !shell.PromptFocused() {
		t.Fatal("new shell did not start with prompt focus")
	}
	if !shell.Handle(input.Key{Code: input.Tab}) || !shell.TranscriptFocused() {
		t.Fatal("Tab did not move focus to the transcript")
	}
	focused := drawRoot(t, shell, 96, 20)
	for _, want := range []string{"select prev", "select next", "toggle details", "prompt"} {
		if !strings.Contains(focused, want) {
			t.Errorf("transcript help does not contain %q:\n%s", want, focused)
		}
	}
	if !shell.Handle(input.Key{Code: input.Character, Rune: ' '}) || !shell.PromptFocused() {
		t.Fatal("Space did not return focus to the prompt")
	}
}

type staticDrawer interface {
	Draw(grid.View)
}

func drawStatic(t *testing.T, widget staticDrawer, width, height int) string {
	t.Helper()
	surface := grid.NewSurface(width, height)
	widget.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}

func drawRoot(t *testing.T, widget headless.Widget, width, height int) string {
	t.Helper()
	surface := grid.NewSurface(width, height)
	headless.NewRoot(widget).Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}
