// Parent module — owns only the package-level doc (doc.go). The three
// reader implementations (markdown / html / pdf) each live in their
// own sub-module so users only pull the parser deps they actually
// need.
module github.com/Tangerg/lynx/documentreaders

go 1.26.4

require (
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
)
