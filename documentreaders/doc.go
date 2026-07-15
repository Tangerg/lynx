// Package documentreaders is the parent of the lynx document-reader
// sibling packages — markdown / html / pdf — that turn external file
// formats into [document.Document] streams suitable for RAG ingestion.
//
// Each sub-module is its own go module so heavy dependencies (PDF
// parser, HTML parser, markdown AST) only land in user binaries that
// actually need them. Pick the readers you want via direct import:
//
//	import (
//	    "github.com/Tangerg/lynx/documentreaders/markdown"
//	    "github.com/Tangerg/lynx/documentreaders/html"
//	    "github.com/Tangerg/lynx/documentreaders/pdf"
//	)
//
// All readers expose a Read method and emit ordinary [*document.Document]
// values. Downstream splitters, embedders, and vector stores therefore see no
// difference between the text/JSON readers in this package and the
// external-format readers.
package documentreaders
