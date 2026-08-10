package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestUserMessageBlockUsesAQuietSurfaceWithoutChangingCopiedText(t *testing.T) {
	theme := kit.Dark()
	block := newUserMessageBlock(theme, "first line\nsecond line")
	width, height := 28, block.Measure(28)
	surface := grid.NewSurface(width, height)
	block.Draw(surface.View())

	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"you", "first line", "second line"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("user message does not contain %q:\n%s", want, drawn)
		}
	}
	padding, ok := surface.CellAt(0, 1)
	if !ok || padding.Style != theme.Surface {
		t.Fatalf("user message padding style = %+v, want surface %+v", padding.Style, theme.Surface)
	}

	rows := block.Rows(width)
	if len(rows) != height {
		t.Fatalf("copied rows = %d, measured height = %d", len(rows), height)
	}
	if rows[0].Text != "you" || rows[0].Offset != userMessageInset {
		t.Fatalf("copied speaker row = %+v", rows[0])
	}
	if rows[1].Text != "first line" || rows[1].Offset != userMessageInset+2 {
		t.Fatalf("copied body row = %+v", rows[1])
	}
}

func TestUserMessageBlockDegradesWithoutLosingTextAtMinimalWidth(t *testing.T) {
	block := newUserMessageBlock(kit.Dark(), "x")
	for _, width := range []int{1, 2} {
		height := block.Measure(width)
		surface := grid.NewSurface(width, height)
		block.Draw(surface.View())
		if rows := block.Rows(width); len(rows) != height {
			t.Fatalf("width %d copied rows = %d, measured height = %d", width, len(rows), height)
		}
	}
}

func TestUserPresenterKeepsAttachmentsInsideTheMessageSurface(t *testing.T) {
	rendered := presentUser(Presentation{Theme: kit.Dark()}, client.Block{
		Kind:        client.BlockUser,
		Text:        "review this",
		Attachments: []client.Attachment{{Name: "design.md", MimeType: "text/markdown"}},
	})
	if len(rendered) != 1 {
		t.Fatalf("presented blocks = %d, want 1", len(rendered))
	}
	message, ok := rendered[0].(*userMessageBlock)
	if !ok || !strings.Contains(message.message.Body, "@design.md · text/markdown") {
		t.Fatalf("presented user message = %#v", rendered[0])
	}
}
