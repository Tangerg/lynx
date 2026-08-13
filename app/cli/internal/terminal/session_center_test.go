package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestSessionCenterRejectsActionsAfterItsControllerCloses(t *testing.T) {
	active := true
	favorites := 0
	center := newSessionCenterPane(kit.Dark(), kit.Unicode(), func(agent.Session) {})
	center.active = func() bool { return active }
	center.toggleFavorite = func(agent.Session) { favorites++ }
	if err := center.SetPage(agent.SessionPage{Items: []agent.Session{{ID: "ses_1", Title: "One"}}}, false); err != nil {
		t.Fatal(err)
	}
	center.Focus(true)
	root := headless.NewRoot(center)
	surface := grid.NewSurface(80, 20)
	root.Draw(surface.View())

	active = false
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	if favorites != 0 {
		t.Fatalf("closed center executed %d favorite actions", favorites)
	}
	active = true
	root.Handle(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	if favorites != 1 {
		t.Fatalf("open center executed %d favorite actions", favorites)
	}
}

func TestSessionCenterRejectsCursorCyclesAcrossUserLoadedPages(t *testing.T) {
	t.Parallel()
	center := newSessionCenterPane(kit.Theme{}, kit.Glyphs{}, func(agent.Session) {})
	center.Reset()
	for _, page := range []agent.SessionPage{{NextCursor: "first"}, {NextCursor: "second"}} {
		if err := center.SetPage(page, center.Cursor() != ""); err != nil {
			t.Fatalf("SetPage(%q): %v", page.NextCursor, err)
		}
	}
	err := center.SetPage(agent.SessionPage{NextCursor: "first"}, true)
	if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
		t.Fatalf("SetPage cycle error = %v", err)
	}
}
