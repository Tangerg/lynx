package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// This file holds the minimal slice of the Language Server Protocol wire
// shapes scopeapp consumes — definition / references / hover / symbols /
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

// DiagnosticSeverity is the closed LSP diagnostic-severity vocabulary.
type DiagnosticSeverity uint8

const (
	DiagnosticSeverityUnspecified DiagnosticSeverity = iota
	DiagnosticSeverityError
	DiagnosticSeverityWarning
	DiagnosticSeverityInformation
	DiagnosticSeverityHint
)

// IsProblem reports whether the severity belongs in the post-mutation problem
// summary. LSP treats an omitted severity as an error for this purpose.
func (d DiagnosticSeverity) IsProblem() bool {
	return d <= DiagnosticSeverityWarning
}

func (d DiagnosticSeverity) name() string {
	switch d {
	case DiagnosticSeverityError:
		return "error"
	case DiagnosticSeverityWarning:
		return "warning"
	case DiagnosticSeverityInformation:
		return "info"
	case DiagnosticSeverityHint:
		return "hint"
	default:
		return ""
	}
}

// Diagnostic is one problem a server reports for a document (a compile error,
// a vet warning).
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// SeverityName renders a Diagnostic.Severity as a word (empty when unset).
func (d Diagnostic) SeverityName() string {
	return d.Severity.name()
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

// configurationItem is the server's request for one settings scope. Scope has
// no LSP-specific settings, but decoding the shape still prevents malformed
// requests from being acknowledged as valid configuration queries.
type configurationItem struct {
	ScopeURI string `json:"scopeUri,omitempty"`
	Section  string `json:"section,omitempty"`
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
func (c callHierarchyItem) symbol() Symbol {
	return Symbol{
		Name:     c.Name,
		Kind:     c.Kind,
		Detail:   c.Detail,
		Location: Location{URI: c.URI, Range: c.SelectionRange},
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
// the document version the server diagnosed, so a post-mutation wait can tell
// fresh diagnostics from stale ones.
type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (p publishDiagnosticsParams) validate() error {
	if strings.TrimSpace(p.URI) == "" {
		return errors.New("uri is empty")
	}
	for index, diagnostic := range p.Diagnostics {
		if err := diagnostic.validate(); err != nil {
			return fmt.Errorf("diagnostic %d: %w", index, err)
		}
	}
	return nil
}

func (d Diagnostic) validate() error {
	if d.Severity > DiagnosticSeverityHint {
		return fmt.Errorf("unknown severity %d", d.Severity)
	}
	if err := d.Range.validate(); err != nil {
		return fmt.Errorf("range: %w", err)
	}
	return nil
}

func (r Range) validate() error {
	if err := r.Start.validate(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := r.End.validate(); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if r.End.Line < r.Start.Line || (r.End.Line == r.Start.Line && r.End.Character < r.Start.Character) {
		return errors.New("end precedes start")
	}
	return nil
}

func (p Position) validate() error {
	if p.Line < 0 {
		return errors.New("line is negative")
	}
	if p.Character < 0 {
		return errors.New("character is negative")
	}
	return nil
}

// --- response shapes we parse flexibly ---

type symbolInformation struct {
	Name          string    `json:"name"`
	Kind          int       `json:"kind"`
	Location      *Location `json:"location"`
	ContainerName string    `json:"containerName"`
}

// locationLink is the alternate definition/implementation result shape. The
// target selection range is the most precise navigation span; targetRange is
// retained only because it is required on the wire.
type locationLink struct {
	TargetURI            *string `json:"targetUri"`
	TargetRange          *Range  `json:"targetRange"`
	TargetSelectionRange *Range  `json:"targetSelectionRange"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail"`
	Kind           int              `json:"kind"`
	Range          *Range           `json:"range"`
	SelectionRange *Range           `json:"selectionRange"`
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
// may be null, a Location, Location[], or LocationLink[]. Unknown and malformed
// shapes are errors rather than false "no result" responses.
func parseLocations(raw json.RawMessage) ([]Location, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return nil, errors.New("decode locations: response is empty")
	}
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}
	switch value[0] {
	case '{':
		location, err := parseLocation(value)
		if err != nil {
			return nil, err
		}
		return []Location{location}, nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(value, &items); err != nil {
			return nil, fmt.Errorf("decode location list: %w", err)
		}
		locations := make([]Location, 0, len(items))
		for index, item := range items {
			location, err := parseLocationItem(item)
			if err != nil {
				return nil, fmt.Errorf("decode location list item %d: %w", index, err)
			}
			locations = append(locations, location)
		}
		return locations, nil
	default:
		return nil, fmt.Errorf("decode locations: expected object, array, or null, got %s", jsonKind(value))
	}
}

func parseLocationItem(raw json.RawMessage) (Location, error) {
	var shape struct {
		URI       *string `json:"uri"`
		TargetURI *string `json:"targetUri"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return Location{}, err
	}
	switch {
	case shape.URI != nil && shape.TargetURI == nil:
		return parseLocation(raw)
	case shape.URI == nil && shape.TargetURI != nil:
		var link locationLink
		if err := json.Unmarshal(raw, &link); err != nil {
			return Location{}, err
		}
		if link.TargetRange == nil {
			return Location{}, errors.New("location link targetRange is missing or null")
		}
		if link.TargetSelectionRange == nil {
			return Location{}, errors.New("location link targetSelectionRange is missing or null")
		}
		location := Location{URI: *link.TargetURI, Range: *link.TargetSelectionRange}
		if err := validateLocation(location); err != nil {
			return Location{}, fmt.Errorf("location link: %w", err)
		}
		return location, nil
	case shape.URI != nil && shape.TargetURI != nil:
		return Location{}, errors.New("ambiguous location object contains both uri and targetUri")
	default:
		return Location{}, errors.New("location object contains neither uri nor targetUri")
	}
}

func parseLocation(raw json.RawMessage) (Location, error) {
	var wire struct {
		URI   *string `json:"uri"`
		Range *Range  `json:"range"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Location{}, fmt.Errorf("decode location: %w", err)
	}
	if wire.URI == nil {
		return Location{}, errors.New("location uri is missing or null")
	}
	if wire.Range == nil {
		return Location{}, errors.New("location range is missing or null")
	}
	location := Location{URI: *wire.URI, Range: *wire.Range}
	if err := validateLocation(location); err != nil {
		return Location{}, err
	}
	return location, nil
}

func validateLocation(location Location) error {
	if strings.TrimSpace(location.URI) == "" {
		return errors.New("location uri is empty")
	}
	return nil
}

// parseSymbols normalizes textDocument/documentSymbol, which is either a flat
// []SymbolInformation (each with a Location) or a hierarchical
// []DocumentSymbol (ranges only — fileURI supplies the location). docURI is
// used to locate hierarchical symbols.
func parseSymbols(raw json.RawMessage, docURI string) ([]Symbol, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return nil, errors.New("decode symbols: response is empty")
	}
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("decode symbols: expected array or null, got %s", jsonKind(value))
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil {
		return nil, fmt.Errorf("decode symbol list: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	var first struct {
		Location json.RawMessage `json:"location"`
	}
	if err := json.Unmarshal(items[0], &first); err != nil {
		return nil, fmt.Errorf("decode first symbol shape: %w", err)
	}
	if len(first.Location) != 0 {
		var infos []symbolInformation
		if err := json.Unmarshal(value, &infos); err != nil {
			return nil, fmt.Errorf("decode SymbolInformation list: %w", err)
		}
		out := make([]Symbol, 0, len(infos))
		for index, symbol := range infos {
			if strings.TrimSpace(symbol.Name) == "" {
				return nil, fmt.Errorf("decode SymbolInformation item %d: name is empty", index)
			}
			if symbol.Location == nil {
				return nil, fmt.Errorf("decode SymbolInformation item %d: location is missing or null", index)
			}
			if err := validateLocation(*symbol.Location); err != nil {
				return nil, fmt.Errorf("decode SymbolInformation item %d: %w", index, err)
			}
			out = append(out, Symbol{Name: symbol.Name, Kind: symbol.Kind, Location: *symbol.Location, Container: symbol.ContainerName})
		}
		return out, nil
	}
	if strings.TrimSpace(docURI) == "" {
		return nil, errors.New("decode DocumentSymbol list: document uri is empty")
	}
	var tree []documentSymbol
	if err := json.Unmarshal(value, &tree); err != nil {
		return nil, fmt.Errorf("decode DocumentSymbol list: %w", err)
	}
	var out []Symbol
	if err := appendDocumentSymbols(&out, "", docURI, tree); err != nil {
		return nil, err
	}
	return out, nil
}

func appendDocumentSymbols(out *[]Symbol, parent, docURI string, symbols []documentSymbol) error {
	for index, symbol := range symbols {
		if strings.TrimSpace(symbol.Name) == "" {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: name is empty", index, parent)
		}
		if symbol.Range == nil {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: range is missing or null", index, parent)
		}
		if symbol.SelectionRange == nil {
			return fmt.Errorf("decode DocumentSymbol item %d under %q: selectionRange is missing or null", index, parent)
		}
		*out = append(*out, Symbol{
			Name:      symbol.Name,
			Kind:      symbol.Kind,
			Detail:    symbol.Detail,
			Location:  Location{URI: docURI, Range: *symbol.SelectionRange},
			Container: parent,
		})
		if err := appendDocumentSymbols(out, symbol.Name, docURI, symbol.Children); err != nil {
			return err
		}
	}
	return nil
}

// hoverText flattens a Hover.contents payload — MarkupContent {kind, value},
// a bare MarkedString, or an array of either — into plain text.
func parseHover(raw json.RawMessage) (string, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return "", errors.New("decode hover: response is empty")
	}
	if bytes.Equal(value, []byte("null")) {
		return "", nil
	}
	if value[0] != '{' {
		return "", fmt.Errorf("decode hover: expected object or null, got %s", jsonKind(value))
	}
	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(value, &hover); err != nil {
		return "", fmt.Errorf("decode hover object: %w", err)
	}
	if len(hover.Contents) == 0 {
		return "", errors.New("decode hover object: contents is missing")
	}
	return hoverText(hover.Contents)
}

func hoverText(raw json.RawMessage) (string, error) {
	value := normalizedJSON(raw)
	if len(value) == 0 {
		return "", errors.New("decode hover contents: value is empty")
	}
	switch value[0] {
	case '"':
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return "", fmt.Errorf("decode hover string: %w", err)
		}
		return strings.TrimSpace(text), nil
	case '{':
		var content struct {
			Value *string `json:"value"`
		}
		if err := json.Unmarshal(value, &content); err != nil {
			return "", fmt.Errorf("decode hover object: %w", err)
		}
		if content.Value == nil {
			return "", errors.New("decode hover object: value is missing or null")
		}
		return strings.TrimSpace(*content.Value), nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(value, &items); err != nil {
			return "", fmt.Errorf("decode hover list: %w", err)
		}
		parts := make([]string, 0, len(items))
		for index, item := range items {
			text, err := hoverText(item)
			if err != nil {
				return "", fmt.Errorf("decode hover list item %d: %w", index, err)
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n"), nil
	default:
		return "", fmt.Errorf("decode hover contents: expected string, object, or array, got %s", jsonKind(value))
	}
}

func normalizedJSON(raw json.RawMessage) []byte {
	return bytes.TrimSpace(raw)
}

func jsonKind(value []byte) string {
	if len(value) == 0 {
		return "empty input"
	}
	switch value[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case 'n':
		return "null"
	case 't', 'f':
		return "boolean"
	case '"':
		return "string"
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-':
		return "number"
	default:
		return fmt.Sprintf("token %q", value[0])
	}
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
