// The parent module owns dependency-light text and JSON readers. Markdown,
// HTML, and PDF readers remain separate modules so users only pull parser
// dependencies they need.
module github.com/Tangerg/lynx/documentreaders

go 1.26.5

require (
	github.com/Tangerg/lynx/core v0.0.0-20260715032326-b968e20dd6f6
	github.com/Tangerg/lynx/pkg v0.0.0-20260715032326-b968e20dd6f6
)
