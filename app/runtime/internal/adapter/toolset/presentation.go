package toolset

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Presenter owns the client-facing projection of concrete tool schemas. Its
// zero value is ready for use. Unknown tools retain their canonical result and
// let the execution adapter supply generic activity text.
type Presenter struct{}

// Activity returns concise progress text for a known concrete tool.
func (Presenter) Activity(name string, arguments tool.Arguments) string {
	descriptor, ok := descriptorFor(name)
	if !ok {
		return ""
	}
	if descriptor.activity != nil {
		return descriptor.activity(arguments)
	}
	return descriptor.activityText
}

func shellToolActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[shellActivityArguments](arguments, "description"); ok &&
		isConciseActivityText(args.Description, 120) {
		return args.Description
	}
	return "Running command"
}

func delegationToolActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[delegationActivityArguments](arguments, "summary"); ok &&
		isConciseActivityText(args.Summary, 80) {
		return "Delegating: " + args.Summary
	}
	return "Delegating to a sub-agent"
}

func createScheduleActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[scheduleActivityArguments](arguments, "title"); ok &&
		isConciseActivityText(args.Title, 120) {
		return "Creating schedule: " + args.Title
	}
	return "Creating a schedule"
}

func loadSkillActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[skillActivityArguments](arguments, "name"); ok &&
		isConciseActivityText(args.Name, 80) {
		return "Loading Skill: " + args.Name
	}
	return "Loading a Skill"
}

func proposeSkillActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[skillActivityArguments](arguments, "name"); ok &&
		isConciseActivityText(args.Name, 64) {
		return "Proposing Skill: " + args.Name
	}
	return "Proposing a Skill"
}

func lspToolActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[lspActivityArguments](arguments, "operation"); ok {
		if activity := lspActivity(args.Operation); activity != "" {
			return activity
		}
	}
	return "Querying the language server"
}

func httpToolActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[httpActivityArguments](arguments, "url"); ok {
		if method := httpMethod(args.Method); method != "" {
			return "Sending " + method + " request"
		}
	}
	return "Sending an HTTP request"
}

type shellActivityArguments struct {
	Description string `json:"description"`
}

type delegationActivityArguments struct {
	Summary string `json:"summary"`
}

type scheduleActivityArguments struct {
	Title string `json:"title"`
}

type skillActivityArguments struct {
	Name string `json:"name"`
}

type lspActivityArguments struct {
	Operation string `json:"operation"`
}

type httpActivityArguments struct {
	URL    string `json:"url"`
	Method string `json:"method"`
}

func isConciseActivityText(value string, maxRunes int) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.RuneCountInString(value) <= maxRunes
}

func lspActivity(operation string) string {
	switch operation {
	case "definition":
		return "Finding a symbol definition"
	case "references":
		return "Finding symbol references"
	case "implementation":
		return "Finding symbol implementations"
	case "hover":
		return "Inspecting a symbol"
	case "incoming_calls":
		return "Finding incoming calls"
	case "outgoing_calls":
		return "Finding outgoing calls"
	case "document_symbols":
		return "Listing document symbols"
	case "workspace_symbols":
		return "Searching workspace symbols"
	case "diagnostics":
		return "Checking file diagnostics"
	default:
		return ""
	}
}

func httpMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "GET"
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE":
		return method
	default:
		return ""
	}
}

// Present projects a known tool's canonical arguments and result into the
// client transcript shape. The second result is optional plain output for
// clients that render command text separately.
func (Presenter) Present(name string, arguments tool.Arguments, result tool.Result) (tool.Result, string) {
	descriptor, ok := descriptorFor(name)
	if !ok || descriptor.presentation == nil {
		return result, ""
	}
	return descriptor.presentation(arguments, result)
}

func presentCommand(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentCommandResult(result)
}

func presentSearch(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentSearchResult(result), ""
}

func presentWebSearch(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentWebSearchResult(result), ""
}

func presentApplyPatch(_ tool.Arguments, result tool.Result) (tool.Result, string) {
	return presentApplyPatchResult(result), ""
}

type commandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode *int   `json:"exit_code"`
}

type commandPresentation struct {
	Output   string `json:"output"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

func presentCommandResult(result tool.Result) (tool.Result, string) {
	if existing, ok := decodeResult[commandPresentation](result, "output"); ok {
		return result, existing.Output
	}
	raw, ok := decodeResult[commandResult](result, "stdout", "stderr", "exit_code")
	if !ok {
		return result, ""
	}
	output := raw.Stdout
	switch {
	case raw.Stdout == "":
		output = raw.Stderr
	case raw.Stderr != "":
		output = raw.Stdout + "\n" + raw.Stderr
	}
	presentation := commandPresentation{Output: output, ExitCode: raw.ExitCode}
	return projectResult(result, presentation), output
}

type localSearchResult struct {
	Matches []localSearchMatch `json:"matches"`
	Files   []string           `json:"files"`
	Paths   []string           `json:"paths"`
	Counts  []localSearchCount `json:"counts"`
}

type localSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type localSearchCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type searchPresentation struct {
	Hits []searchHit `json:"hits"`
}

type searchHit struct {
	Path       string `json:"path"`
	Snippet    string `json:"snippet,omitempty"`
	LineNumber int    `json:"lineNumber,omitempty"`
}

func presentSearchResult(result tool.Result) tool.Result {
	if _, ok := decodeResult[searchPresentation](result, "hits"); ok {
		return result
	}
	raw, ok := decodeResult[localSearchResult](result, "matches", "files", "paths", "counts")
	if !ok {
		return result
	}
	hits := make([]searchHit, 0, len(raw.Matches)+len(raw.Files)+len(raw.Paths)+len(raw.Counts))
	for _, match := range raw.Matches {
		hits = append(hits, searchHit{Path: match.Path, Snippet: match.Text, LineNumber: match.Line})
	}
	for _, path := range raw.Files {
		hits = append(hits, searchHit{Path: path})
	}
	for _, path := range raw.Paths {
		hits = append(hits, searchHit{Path: path})
	}
	for _, count := range raw.Counts {
		hits = append(hits, searchHit{Path: count.Path, Snippet: strconv.Itoa(count.Count) + " matches"})
	}
	return projectResult(result, searchPresentation{Hits: hits})
}

type webSearchResult struct {
	Results []webSearchHit `json:"results"`
}

type webSearchHit struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	Snippet        string `json:"snippet"`
	FaviconURL     string `json:"favicon_url"`
	FaviconURLWire string `json:"faviconUrl"`
}

type webSearchPresentation struct {
	Results []webSearchPresentationHit `json:"results"`
}

type webSearchPresentationHit struct {
	Title      string `json:"title,omitempty"`
	URL        string `json:"url"`
	Snippet    string `json:"snippet,omitempty"`
	FaviconURL string `json:"faviconUrl,omitempty"`
}

func presentWebSearchResult(result tool.Result) tool.Result {
	raw, ok := decodeResult[webSearchResult](result, "results")
	if !ok {
		return result
	}
	items := make([]webSearchPresentationHit, 0, len(raw.Results))
	for _, item := range raw.Results {
		faviconURL := item.FaviconURL
		if faviconURL == "" {
			faviconURL = item.FaviconURLWire
		}
		items = append(items, webSearchPresentationHit{
			Title: item.Title, URL: item.URL, Snippet: item.Snippet, FaviconURL: faviconURL,
		})
	}
	return projectResult(result, webSearchPresentation{Results: items})
}

type changePresentation struct {
	Changes []presentedChange `json:"changes"`
}

type changeStatus string

const (
	changeAdded    changeStatus = "added"
	changeDeleted  changeStatus = "deleted"
	changeModified changeStatus = "modified"
	changeMoved    changeStatus = "moved"
)

type presentedChange struct {
	Path   string       `json:"path"`
	Status changeStatus `json:"status"`
	From   string       `json:"from,omitempty"`
}

type applyPatchResult struct {
	Files []struct {
		Path      string `json:"path"`
		Created   bool   `json:"created"`
		Deleted   bool   `json:"deleted"`
		MovedFrom string `json:"moved_from"`
	} `json:"files"`
}

func presentApplyPatchResult(result tool.Result) tool.Result {
	if _, ok := decodeResult[changePresentation](result, "changes"); ok {
		return result
	}
	decoded, ok := decodeResult[applyPatchResult](result, "files")
	if !ok || len(decoded.Files) == 0 {
		return result
	}
	changes := make([]presentedChange, 0, len(decoded.Files))
	for _, file := range decoded.Files {
		status := changeModified
		switch {
		case file.MovedFrom != "":
			status = changeMoved
		case file.Created:
			status = changeAdded
		case file.Deleted:
			status = changeDeleted
		}
		changes = append(changes, presentedChange{
			Path: file.Path, Status: status, From: file.MovedFrom,
		})
	}
	return projectResult(result, changePresentation{Changes: changes})
}

func decodeResult[T any](result tool.Result, knownFields ...string) (T, bool) {
	data, err := json.Marshal(result)
	if err != nil {
		return *new(T), false
	}
	return decodePresentation[T](data, knownFields...)
}

func decodeArguments[T any](arguments tool.Arguments, knownFields ...string) (T, bool) {
	return decodePresentation[T]([]byte(arguments.Canonical()), knownFields...)
}

func decodePresentation[T any](data []byte, knownFields ...string) (T, bool) {
	var decoded T
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return decoded, false
	}
	for _, field := range knownFields {
		if _, ok := fields[field]; !ok {
			continue
		}
		if json.Unmarshal(data, &decoded) == nil {
			return decoded, true
		}
		return decoded, false
	}
	return decoded, false
}

type presentationValue interface {
	commandPresentation | searchPresentation | webSearchPresentation | changePresentation
}

func projectResult[T presentationValue](original tool.Result, value T) tool.Result {
	projected, err := tool.NewResult(value)
	if err != nil {
		return original
	}
	return projected
}
