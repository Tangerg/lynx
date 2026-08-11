package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

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
