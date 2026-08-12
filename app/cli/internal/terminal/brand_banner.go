package terminal

import (
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const brandMarkMinWidth = 40

var (
	unicodeBrandMark = []string{
		"██╗     ██╗   ██╗██████╗  █████╗",
		"██║     ╚██╗ ██╔╝██╔══██╗██╔══██╗",
		"██║      ╚████╔╝ ██████╔╝███████║",
		"██║       ╚██╔╝  ██╔══██╗██╔══██║",
		"███████╗   ██║   ██║  ██║██║  ██║",
		"╚══════╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝",
	}
	asciiBrandMark = []string{
		" _      __   __  ____      _",
		"| |     \\ \\ / / |  _ \\    / \\",
		"| |      \\ V /  | |_) |  / _ \\",
		"| |___    | |   |  _ <  / ___ \\",
		"|_____|   |_|   |_| \\_\\/_/   \\_\\",
	}
)

// brandBanner is the empty-session projection. It is intentionally outside the
// transcript model so identity chrome never becomes searchable conversation data.
type brandBanner struct {
	theme     kit.Theme
	glyphs    kit.Glyphs
	version   string
	model     string
	workspace string
}

var _ grid.Drawable = (*brandBanner)(nil)

type brandLine struct {
	text  string
	style grid.Style
}

func newBrandBanner(
	theme kit.Theme,
	glyphs kit.Glyphs,
	version string,
	session agent.Session,
	options agent.RunOptions,
) *brandBanner {
	banner := &brandBanner{theme: theme, glyphs: glyphs, version: brandVersion(version)}
	banner.SetSession(session)
	banner.SetOptions(options)
	return banner
}

func (b *brandBanner) SetSession(session agent.Session) {
	if b != nil {
		b.workspace = displayWorkspace(session.Workspace)
	}
}

func (b *brandBanner) SetOptions(options agent.RunOptions) {
	if b != nil {
		b.model = modelLabel(options)
	}
}

func (b *brandBanner) Measure(width int) int {
	if b == nil || width <= 0 {
		return 0
	}
	return len(b.lines(width, len(unicodeBrandMark)+4))
}

func (b *brandBanner) Draw(view grid.View) {
	if b == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	lines := b.lines(width, height)
	top := max((height-len(lines))/3, 0)
	for row, line := range lines {
		value := text.Truncate(line.text, width, b.glyphs.Ellipsis)
		x := max((width-text.Width(value))/2, 0)
		view.Text(x, top+row, value, line.style)
	}
}

func (b *brandBanner) lines(width, height int) []brandLine {
	if width <= 0 || height <= 0 {
		return nil
	}
	if width < brandMarkMinWidth || height < len(asciiBrandMark)+1 {
		return b.compactLines(height)
	}

	mark := unicodeBrandMark
	if b.glyphs.Horizontal == "-" {
		mark = asciiBrandMark
	}
	lines := make([]brandLine, 0, len(mark)+4)
	for _, line := range mark {
		lines = append(lines, brandLine{text: line, style: b.theme.Accent})
	}
	if len(lines) < height-2 {
		lines = append(lines, brandLine{})
	}
	return append(lines, b.detailLines(height-len(lines))...)
}

func (b *brandBanner) compactLines(height int) []brandLine {
	lines := []brandLine{{text: "LYRA", style: b.theme.Accent}}
	return append(lines, b.detailLines(height-len(lines))...)
}

func (b *brandBanner) detailLines(capacity int) []brandLine {
	if capacity <= 0 {
		return nil
	}
	candidates := []brandLine{
		{text: "Lyra CLI  " + b.version, style: b.theme.Heading},
		{text: b.model, style: b.theme.Muted},
		{text: b.workspace, style: b.theme.Subtle},
	}
	return candidates[:min(capacity, len(candidates))]
}

func brandVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "dev" || strings.HasPrefix(value, "v") {
		if value == "" {
			return "dev"
		}
		return value
	}
	return "v" + value
}
