package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ServerSpec describes how to launch and address one language server.
//
// Adding support for a new language is exactly this: append one ServerSpec to
// the table — no other code changes. The client, server set, and tools are all
// language-agnostic; they drive whatever specs they're handed.
type ServerSpec struct {
	// Name identifies the server (gopls, typescript-language-server). It keys
	// the per-(workspace-root, server) client, so it must be unique.
	Name string

	// Command and Args launch the server, which must speak LSP over stdio.
	Command string
	Args    []string

	// LanguageID is the LSP languageId reported in didOpen (go, typescript).
	LanguageID string

	// Extensions are the file suffixes this server handles (".go"). They route
	// file-anchored operations (definition, hover) to the right server.
	Extensions []string

	// RootMarkers are filenames whose presence at a workspace root signals this
	// language applies there (go.mod, package.json). They route workspace-wide
	// operations (workspace symbol) that aren't anchored to a single file.
	RootMarkers []string
}

// DefaultServers is the built-in server table — the languages lyra supports
// out of the box. Every entry is self-contained, so adding a language is a
// single literal here (or a config override; see engine.Config.LSPServers).
// A server whose Command isn't installed simply stays unavailable: its files
// resolve no server and the tools report that, nothing crashes.
func DefaultServers() []ServerSpec {
	return []ServerSpec{
		{
			Name:        "gopls",
			Command:     "gopls",
			LanguageID:  "go",
			Extensions:  []string{".go"},
			RootMarkers: []string{"go.mod", "go.work"},
		},
		{
			Name:        "typescript",
			Command:     "typescript-language-server",
			Args:        []string{"--stdio"},
			LanguageID:  "typescript",
			Extensions:  []string{".ts", ".tsx", ".mts", ".cts"},
			RootMarkers: []string{"tsconfig.json", "jsconfig.json", "package.json"},
		},
	}
}

// serverTable indexes a set of specs for the two routing questions the server set
// asks: which server handles this file, and which servers apply to this root.
type serverTable struct {
	specs []ServerSpec
	byExt map[string]ServerSpec
}

func newServerTable(specs []ServerSpec) *serverTable {
	specs = slices.Clone(specs)
	for i := range specs {
		specs[i].Args = slices.Clone(specs[i].Args)
		specs[i].Extensions = slices.Clone(specs[i].Extensions)
		specs[i].RootMarkers = slices.Clone(specs[i].RootMarkers)
	}
	byExt := make(map[string]ServerSpec, len(specs))
	for _, spec := range specs {
		for _, ext := range spec.Extensions {
			byExt[strings.ToLower(ext)] = spec
		}
	}
	return &serverTable{specs: specs, byExt: byExt}
}

// forFile returns the server that handles path's extension.
func (t *serverTable) forFile(path string) (ServerSpec, bool) {
	spec, ok := t.byExt[strings.ToLower(filepath.Ext(path))]
	return spec, ok
}

// forRoot returns the servers whose root markers exist directly under root —
// the languages that apply to this workspace.
func (t *serverTable) forRoot(root string) []ServerSpec {
	var out []ServerSpec
	for _, spec := range t.specs {
		for _, marker := range spec.RootMarkers {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				out = append(out, spec)
				break
			}
		}
	}
	return out
}
