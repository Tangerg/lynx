package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestSessionHeaderUsesSpaceProgressively(t *testing.T) {
	header := newSessionHeader(kit.Dark(), kit.Unicode(), agent.Session{
		Title:     "Architecture review",
		Workspace: "/workspace/lynx",
	})
	header.SetUsage(agent.Usage{InputTokens: 1_234, OutputTokens: 56_789})

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

func TestStatusProgressIncludesRuntimeActivityStepAndContext(t *testing.T) {
	status := newStatusView(kit.Dark(), kit.Unicode(), settings.Default().RunOptions())
	step, contextTokens := 7, int64(12_345)
	status.progress(agent.RunProgress{Step: &step, ContextTokens: &contextTokens, Activity: "calling tools"})
	if !status.busy || status.doing != "calling tools · step 7 · ctx 12,345" {
		t.Fatalf("progress status = busy %t, doing %q", status.busy, status.doing)
	}
}

func TestActivityViewCentersACompactWindowOnTheActiveStep(t *testing.T) {
	activity := newActivityView(kit.Dark(), kit.Unicode())
	activity.Set([]agent.PlanItem{
		{Title: "Inspect references", Status: agent.PlanDone},
		{Title: "Define shell", Status: agent.PlanDone},
		{Title: "Build prompt", Status: agent.PlanPending},
		{Title: "Refine tools", Status: agent.PlanActive},
		{Title: "Add responsive tests", Status: agent.PlanPending},
		{Title: "Run quality gates", Status: agent.PlanPending},
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
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	composer := kit.Composer{Theme: kit.Dark(), Prompt: "> "}
	prompt := newPromptView(kit.Dark(), kit.Unicode(), bindings.editor, &composer, settings.Default().RunOptions())
	prompt.Focus(true)

	idle := drawRoot(t, prompt, 120, prompt.Measure(120))
	for _, want := range []string{settings.DefaultProvider + "/" + settings.DefaultModel, "enter", "shift+enter", "ctrl+p"} {
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
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	header := newSessionHeader(theme, glyphs, agent.Session{Title: "New session", Workspace: "/workspace/lynx"})
	activity := newActivityView(theme, glyphs)
	activity.Set([]agent.PlanItem{{Title: "Inspect", Status: agent.PlanActive}})
	status := newStatusView(theme, glyphs, settings.Default().RunOptions())
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	prompt := newPromptView(theme, glyphs, bindings.editor, &composer, settings.Default().RunOptions())
	shell := newShellView(header, transcript, activity, newQueueView(theme, glyphs), status, prompt)
	shell.Focus(true)

	for _, size := range []struct{ width, height int }{
		{96, 28}, {44, 18}, {20, 8}, {11, 20}, {6, 2}, {1, 1},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			got := drawRoot(t, shell, size.width, size.height)
			if strings.TrimSpace(got) == "" {
				t.Fatalf("%dx%d shell rendered nothing", size.width, size.height)
			}
		})
	}
}

func TestShellUsesTwoRowChromeOnTinyTerminals(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	transcript.Append(&kit.Message{Theme: theme, Speaker: "lyra", Body: "VISIBLE_TRANSCRIPT"})
	header := newSessionHeader(theme, glyphs, agent.Session{Title: "Hidden title", Workspace: "/hidden/workspace"})
	activity := newActivityView(theme, glyphs)
	activity.Set([]agent.PlanItem{{Title: "HIDDEN_PLAN", Status: agent.PlanActive}})
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	composer.Editor().Keys = bindings.editor
	composer.Editor().SetText("TINY_DRAFT")
	prompt := newPromptView(theme, glyphs, bindings.editor, &composer, settings.Default().RunOptions())
	shell := newShellView(header, transcript, activity, newQueueView(theme, glyphs), newStatusView(theme, glyphs, settings.Default().RunOptions()), prompt)
	shell.Focus(true)

	tiny := drawRoot(t, shell, 20, compactShellHeight-1)
	if !shell.compact || !prompt.compact {
		t.Fatal("tiny shell did not enter compact layout")
	}
	for _, want := range []string{"VISIBLE", "TINY_DRAFT", "rea"} {
		if !strings.Contains(tiny, want) {
			t.Errorf("tiny shell does not contain %q:\n%s", want, tiny)
		}
	}
	for _, hidden := range []string{"/hidden/workspace", "HIDDEN_PLAN", "shift+enter"} {
		if strings.Contains(tiny, hidden) {
			t.Errorf("tiny shell retained %q:\n%s", hidden, tiny)
		}
	}

	normal := drawRoot(t, shell, 96, 28)
	if shell.compact || prompt.compact {
		t.Fatal("resized shell did not leave compact layout")
	}
	for _, want := range []string{"/hidden/workspace", "HIDDEN_PLAN", "TINY_DRAFT", "shift+enter"} {
		if !strings.Contains(normal, want) {
			t.Errorf("restored shell does not contain %q:\n%s", want, normal)
		}
	}
}

func TestResponsiveShellPreservesTranscriptFocusAndDraft(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	appendTestTool(transcript, "focus", "detail")
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	composer.Editor().Keys = bindings.editor
	composer.Editor().SetText("PRESERVED_DRAFT")
	prompt := newPromptView(theme, glyphs, bindings.editor, &composer, settings.Default().RunOptions())
	shell := newShellView(
		newSessionHeader(theme, glyphs, agent.Session{}), transcript,
		newActivityView(theme, glyphs), newQueueView(theme, glyphs),
		newStatusView(theme, glyphs, settings.Default().RunOptions()), prompt,
	)
	shell.Focus(true)
	if !shell.Handle(input.Key{Code: input.Tab}) || !shell.TranscriptFocused() {
		t.Fatal("test could not focus the transcript")
	}

	drawRoot(t, shell, 20, compactShellHeight-1)
	if !shell.TranscriptFocused() {
		t.Fatal("entering compact layout moved focus away from the transcript")
	}
	drawRoot(t, shell, 96, 28)
	if !shell.TranscriptFocused() {
		t.Fatal("leaving compact layout moved focus away from the transcript")
	}
	if got := composer.Editor().Text(); got != "PRESERVED_DRAFT" {
		t.Fatalf("resize changed draft to %q", got)
	}
}

func TestShellMovesFocusBetweenPromptAndTranscript(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	theme, glyphs := kit.Dark(), kit.Unicode()
	transcript := testTranscriptView(t)
	appendTestTool(transcript, "focus", "detail")
	composer := kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 6}
	prompt := newPromptView(theme, glyphs, bindings.editor, &composer, settings.Default().RunOptions())
	shell := newShellView(
		newSessionHeader(theme, glyphs, agent.Session{}), transcript,
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
