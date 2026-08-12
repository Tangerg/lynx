package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

func TestBrandBannerProjectsBuildModelAndWorkspaceResponsively(t *testing.T) {
	session := agent.Session{Workspace: workspace.Workspace{
		Path: "/workspace/lynx", ProjectRoot: "/workspace", Availability: workspace.Available,
	}}
	banner := newBrandBanner(kit.Dark(), kit.Unicode(), "1.2.3", session, settings.Default().RunOptions())

	wide := drawStatic(t, banner, 96, 14)
	for _, want := range []string{"███████╗", "Lyra CLI  v1.2.3", "deepseek/deepseek-v4-flash", "/workspace/lynx"} {
		if !strings.Contains(wide, want) {
			t.Errorf("wide brand banner does not contain %q:\n%s", want, wide)
		}
	}

	compact := drawStatic(t, banner, brandMarkMinWidth-1, 5)
	for _, want := range []string{"LYRA", "Lyra CLI  v1.2.3", "/workspace/lynx"} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact brand banner does not contain %q:\n%s", want, compact)
		}
	}
	if strings.Contains(compact, "████") {
		t.Fatalf("compact brand banner retained its large mark:\n%s", compact)
	}

	minimal := drawStatic(t, banner, 1, 1)
	if strings.TrimSpace(minimal) == "" {
		t.Fatal("minimal brand banner rendered nothing")
	}
}

func TestLargeBrandMarksFitTheirResponsiveBreakpoint(t *testing.T) {
	for name, mark := range map[string][]string{"unicode": unicodeBrandMark, "ASCII": asciiBrandMark} {
		t.Run(name, func(t *testing.T) {
			for row, line := range mark {
				if width := text.Width(line); width > brandMarkMinWidth {
					t.Fatalf("mark row %d width = %d, exceeds breakpoint %d", row, width, brandMarkMinWidth)
				}
			}
		})
	}
}

func TestBrandBannerUsesASCIIMarkForASCIITerminals(t *testing.T) {
	banner := newBrandBanner(kit.Dark(), kit.ASCII(), "dev", agent.Session{}, agent.RunOptions{})
	got := drawStatic(t, banner, 72, 12)
	if !strings.Contains(got, "\\ V /") || strings.Contains(got, "██") {
		t.Fatalf("ASCII brand banner used the wrong mark:\n%s", got)
	}
}

func TestTranscriptBrandIsAOneShotEntranceProjection(t *testing.T) {
	view := testTranscriptView(t)
	banner := newBrandBanner(kit.Dark(), kit.Unicode(), "test", agent.Session{}, agent.RunOptions{})
	view.SetEntrance(banner)

	if empty := drawRoot(t, view, 72, 12); !strings.Contains(empty, "Lyra CLI  vtest") {
		t.Fatalf("empty transcript does not show the brand:\n%s", empty)
	}
	view.Append(newUserMessageBlock(kit.Dark(), "inspect this repository"))
	filled := drawRoot(t, view, 72, 12)
	if !strings.Contains(filled, "inspect this repository") || strings.Contains(filled, "Lyra CLI") {
		t.Fatalf("conversation did not replace the brand:\n%s", filled)
	}

	view.Reset()
	if cleared := drawRoot(t, view, 72, 12); strings.Contains(cleared, "Lyra CLI") {
		t.Fatalf("reset transcript restored the consumed brand:\n%s", cleared)
	}
}

func TestTranscriptResetConsumesAnUnshownEntranceProjection(t *testing.T) {
	view := testTranscriptView(t)
	view.SetEntrance(newBrandBanner(kit.Dark(), kit.Unicode(), "test", agent.Session{}, agent.RunOptions{}))

	view.Reset()
	if got := drawRoot(t, view, 72, 12); strings.Contains(got, "Lyra CLI") {
		t.Fatalf("reset transcript restored the startup brand:\n%s", got)
	}
}

func TestReplacementTranscriptDoesNotInheritTheBrand(t *testing.T) {
	initial := testTranscriptView(t)
	initial.SetEntrance(newBrandBanner(kit.Dark(), kit.Unicode(), "test", agent.Session{}, agent.RunOptions{}))
	a := &app{transcript: initial, syntax: initial.syntax, settings: settings.Default()}

	replacement := a.newTranscript()
	t.Cleanup(replacement.Close)
	if got := drawRoot(t, replacement, 72, 12); strings.Contains(got, "Lyra CLI") {
		t.Fatalf("replacement transcript inherited the startup brand:\n%s", got)
	}
}
