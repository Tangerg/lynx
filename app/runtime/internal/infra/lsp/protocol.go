package lsp

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

// This file holds the minimal slice of the Language Server Protocol wire
// shapes lyra consumes — definition / references / hover / symbols /
// diagnostics, plus the document-sync notifications a server needs before it
// will answer. It is deliberately NOT the full protocol: we type only what we
// read, and let the rest pass through as json.RawMessage. LSP positions are
// 0-based (line and character); the tool layer converts to/from 1-based.

// Position is a 0-based (line, character) cursor in a document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open [Start, End) span.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a span within a document, addressed by URI.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Path returns the location's native filesystem path (the inverse of the
// file:// URI), for rendering results back to the caller.
func (l Location) Path() string { return uriToPath(l.URI) }

// Diagnostic is one problem a server reports for a document (a compile error,
// a vet warning). Severity: 1=error 2=warning 3=info 4=hint.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     any    `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// SeverityName renders a Diagnostic.Severity as a word (empty when unset).
func (d Diagnostic) SeverityName() string {
	switch d.Severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return ""
	}
}

// Symbol is the normalized form of both LSP symbol shapes
// (SymbolInformation, hierarchical DocumentSymbol) the server set returns to the
// tool layer. Kind is the raw LSP SymbolKind number; Container is the
// enclosing scope when the server reports one.
type Symbol struct {
	Name      string
	Kind      int
	Location  Location
	Container string
	Detail    string
}

// --- params (unexported: only marshaled outward) ---

type initializeParams struct {
	ProcessID        int               `json:"processId"`
	RootURI          string            `json:"rootUri"`
	Capabilities     map[string]any    `json:"capabilities"`
	WorkspaceFolders []workspaceFolder `json:"workspaceFolders,omitempty"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Text string `json:"text"` // full-document sync (the only sync kind we use)
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type positionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type referenceParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type documentSymbolParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type workspaceSymbolParams struct {
	Query string `json:"query"`
}

// callHierarchyItem is one node in the call graph (a function/method), returned
// by prepareCallHierarchy and carried back into incoming/outgoingCalls. We type
// the fields the tool layer renders; the server round-trips the rest opaquely
// via [json.RawMessage] so an item is handed back byte-for-byte.
type callHierarchyItem struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"`
	URI            string          `json:"uri"`
	Range          Range           `json:"range"`
	SelectionRange Range           `json:"selectionRange"`
	Detail         string          `json:"detail,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"` // server-private; preserved across the round trip
}

// symbol maps a call-hierarchy node onto the normalized [Symbol] the tool layer
// formats — its selection range is the precise name span.
func (it callHierarchyItem) symbol() Symbol {
	return Symbol{
		Name:     it.Name,
		Kind:     it.Kind,
		Detail:   it.Detail,
		Location: Location{URI: it.URI, Range: it.SelectionRange},
	}
}

type callHierarchyItemParams struct {
	Item callHierarchyItem `json:"item"`
}

// callHierarchyIncomingCall is one caller (`from`) of the queried symbol;
// outgoing is one callee (`to`). fromRanges (the exact call sites) are not
// rendered — the caller/callee location suffices for navigation.
type callHierarchyIncomingCall struct {
	From callHierarchyItem `json:"from"`
}

type callHierarchyOutgoingCall struct {
	To callHierarchyItem `json:"to"`
}

// publishDiagnosticsParams is the server→client push we cache. Version echoes
// the document version the server diagnosed, so a post-edit wait can tell
// fresh diagnostics from stale ones.
type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// --- response shapes we parse flexibly ---

type symbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []documentSymbol `json:"children"`
}

// defaultCapabilities is the minimal client capability set. The capabilities
// object is a sprawling optional bag, so a map is the honest, low-ceremony
// shape here — we declare only what we use. hierarchicalDocumentSymbolSupport
// is false so documentSymbol comes back as flat SymbolInformation (each
// carries a Location, which is all the tool layer formats).
func defaultCapabilities() map[string]any {
	return map[string]any{
		"textDocument": map[string]any{
			"synchronization":    map[string]any{"dynamicRegistration": false, "didSave": false},
			"definition":         map[string]any{},
			"references":         map[string]any{},
			"implementation":     map[string]any{},
			"hover":              map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
			"documentSymbol":     map[string]any{"hierarchicalDocumentSymbolSupport": false},
			"callHierarchy":      map[string]any{},
			"publishDiagnostics": map[string]any{},
		},
		"workspace": map[string]any{
			"symbol":           map[string]any{},
			"configuration":    true,
			"workspaceFolders": true,
		},
	}
}

// parseLocations normalizes textDocument/definition|references results, which
// may be null, a single Location, or an array of Location.
func parseLocations(raw json.RawMessage) []Location {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var many []Location
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	var one Location
	if err := json.Unmarshal(raw, &one); err == nil && one.URI != "" {
		return []Location{one}
	}
	return nil
}

// parseSymbols normalizes textDocument/documentSymbol, which is either a flat
// []SymbolInformation (each with a Location) or a hierarchical
// []DocumentSymbol (ranges only — fileURI supplies the location). docURI is
// used to locate hierarchical symbols.
func parseSymbols(raw json.RawMessage, docURI string) []Symbol {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var infos []symbolInformation
	if err := json.Unmarshal(raw, &infos); err == nil && len(infos) > 0 && infos[0].Location.URI != "" {
		out := make([]Symbol, 0, len(infos))
		for _, s := range infos {
			out = append(out, Symbol{Name: s.Name, Kind: s.Kind, Location: s.Location, Container: s.ContainerName})
		}
		return out
	}
	var tree []documentSymbol
	if err := json.Unmarshal(raw, &tree); err == nil {
		var out []Symbol
		var walk func(parent string, ds []documentSymbol)
		walk = func(parent string, ds []documentSymbol) {
			for _, s := range ds {
				out = append(out, Symbol{
					Name:      s.Name,
					Kind:      s.Kind,
					Detail:    s.Detail,
					Location:  Location{URI: docURI, Range: s.SelectionRange},
					Container: parent,
				})
				walk(s.Name, s.Children)
			}
		}
		walk("", tree)
		return out
	}
	return nil
}

// hoverText flattens a Hover.contents payload — MarkupContent {kind, value},
// a bare MarkedString, or an array of either — into plain text.
func hoverText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// MarkupContent / MarkedString object: {value: "..."} (or {kind,value}).
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Value != "" {
		return strings.TrimSpace(obj.Value)
	}
	// Bare string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	// Array of strings / objects.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, e := range arr {
			if t := hoverText(e); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

// pathToURI converts an absolute filesystem path to a file:// URI, portable
// across OSes (a Windows C:\a\b becomes file:///C:/a/b). url.URL handles the
// escaping so paths with spaces survive the round trip.
func pathToURI(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // drive-letter (Windows) or relative → leading slash
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// uriToPath is the inverse of pathToURI, used to render results back as
// native filesystem paths.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:] // /C:/a → C:/a (Windows)
	}
	return filepath.FromSlash(p)
}
