// Package documentreaders is the parent of the lynx document-reader
// sibling modules — markdown / html / pdf — that turn external file
// formats into [document.Document] streams suitable for RAG ingestion.
//
// Each sub-module is its own go module so heavy dependencies (PDF
// parser, HTML parser, markdown AST) only land in user binaries that
// actually need them. Pick the readers you want via direct import:
//
//	import (
//	    "github.com/Tangerg/lynx/document-readers/markdown"
//	    "github.com/Tangerg/lynx/document-readers/html"
//	    "github.com/Tangerg/lynx/document-readers/pdf"
//	)
//
// All sub-modules implement [document.Reader] from
// github.com/Tangerg/lynx/core/document and emit ordinary
// [*document.Document] values — downstream splitters, embedders and
// vector stores see no difference between text/JSON readers (in core)
// and these external-format readers.
package documentreaders
