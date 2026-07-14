// The parent module owns dependency-light text and JSON readers. Markdown,
// HTML, and PDF readers remain separate modules so users only pull parser
// dependencies they need.
module github.com/Tangerg/lynx/documentreaders

go 1.26.4

require (
	github.com/Tangerg/lynx/core v0.0.0-20260630070700-be6ce2176c4e
	github.com/Tangerg/lynx/pkg v0.0.0-20260630070700-be6ce2176c4e
)

require (
	github.com/bits-and-blooms/bitset v1.24.5 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkoukk/tiktoken-go v0.1.8 // indirect
	github.com/spf13/cast v1.10.0 // indirect
)
