// Parent module — owns only the package-level doc (doc.go). The three
// reader implementations (markdown / html / pdf) each live in their
// own sub-module so users only pull the parser deps they actually
// need.
module github.com/Tangerg/lynx/documentreaders

go 1.26.4
