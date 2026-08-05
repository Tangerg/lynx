package toolset

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// Presenter owns the client-facing projection of concrete tool schemas. Its
// zero value is ready for use. Unknown tools retain their canonical result and
// let the execution adapter supply generic activity text.
type Presenter struct{}

// PresentationContract describes one built-in tool result after Presenter has
// projected it into the transcript. ResultType is the exact root object carried
// by ToolInvocation.result; EnumValues supplies the closed string vocabularies
// reflection cannot discover.
type PresentationContract struct {
	ToolName   string
	ResultType reflect.Type
	EnumValues map[reflect.Type][]string
}

// PresentationContracts returns the concrete result contracts from the same
// descriptors Presenter executes. Callers receive fresh slices and maps.
func PresentationContracts() []PresentationContract {
	var contracts []PresentationContract
	for name, descriptor := range descriptors() {
		if descriptor.result.project == nil {
			continue
		}
		contract := PresentationContract{ToolName: name, ResultType: descriptor.result.resultType}
		if descriptor.result.enums != nil {
			contract.EnumValues = descriptor.result.enums()
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

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

func shellActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[shellActivityArguments](arguments, "description"); ok &&
		isConciseActivityText(args.Description, 120) {
		return args.Description
	}
	return "Running command"
}

func delegationActivity(arguments tool.Arguments) string {
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

func lspActivity(arguments tool.Arguments) string {
	if args, ok := decodeArguments[lspActivityArguments](arguments, "operation"); ok {
		if activity := lspOperationActivity(args.Operation); activity != "" {
			return activity
		}
	}
	return "Querying the language server"
}

func httpActivity(arguments tool.Arguments) string {
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

func lspOperationActivity(operation string) string {
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
	if !ok || descriptor.result.project == nil {
		return result, ""
	}
	return descriptor.result.project(arguments, result)
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

func commandResultContract() resultProjectionContract {
	return resultProjectionContract{project: presentCommand, resultType: reflect.TypeFor[CommandResult]()}
}

type commandExecutionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode *int   `json:"exit_code"`
}

// CommandResult is the transcript result of shell.
type CommandResult struct {
	Output   string `json:"output"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

func presentCommandResult(result tool.Result) (tool.Result, string) {
	if existing, ok := decodeResult[CommandResult](result, "output"); ok {
		return result, existing.Output
	}
	raw, ok := decodeResult[commandExecutionResult](result, "stdout", "stderr", "exit_code")
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
	presentation := CommandResult{Output: output, ExitCode: raw.ExitCode}
	return projectResult(result, presentation), output
}

func searchResultContract() resultProjectionContract {
	return resultProjectionContract{project: presentSearch, resultType: reflect.TypeFor[SearchResult]()}
}

type localSearchExecutionResult struct {
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

// SearchResult is the transcript result shared by glob and grep.
type SearchResult struct {
	Hits []SearchHit `json:"hits"`
}

// SearchHit identifies one local path and, for content matches, its location
// and preview.
type SearchHit struct {
	Path       string `json:"path"`
	Snippet    string `json:"snippet,omitempty"`
	LineNumber int    `json:"lineNumber,omitempty"`
}

func presentSearchResult(result tool.Result) tool.Result {
	if _, ok := decodeResult[SearchResult](result, "hits"); ok {
		return result
	}
	raw, ok := decodeResult[localSearchExecutionResult](result, "matches", "files", "paths", "counts")
	if !ok {
		return result
	}
	hits := make([]SearchHit, 0, len(raw.Matches)+len(raw.Files)+len(raw.Paths)+len(raw.Counts))
	for _, match := range raw.Matches {
		hits = append(hits, SearchHit{Path: match.Path, Snippet: match.Text, LineNumber: match.Line})
	}
	for _, path := range raw.Files {
		hits = append(hits, SearchHit{Path: path})
	}
	for _, path := range raw.Paths {
		hits = append(hits, SearchHit{Path: path})
	}
	for _, count := range raw.Counts {
		hits = append(hits, SearchHit{Path: count.Path, Snippet: strconv.Itoa(count.Count) + " matches"})
	}
	return projectResult(result, SearchResult{Hits: hits})
}

func webSearchResultContract() resultProjectionContract {
	return resultProjectionContract{project: presentWebSearch, resultType: reflect.TypeFor[WebSearchResult]()}
}

type webSearchExecutionResult struct {
	Results []webSearchExecutionHit `json:"results"`
}

type webSearchExecutionHit struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	Snippet        string `json:"snippet"`
	FaviconURL     string `json:"favicon_url"`
	FaviconURLWire string `json:"faviconUrl"`
}

// WebSearchResult is the transcript result of web_search.
type WebSearchResult struct {
	Results []WebSearchHit `json:"results"`
}

// WebSearchHit is one web_search source suitable for a result card.
type WebSearchHit struct {
	Title      string `json:"title,omitempty"`
	URL        string `json:"url"`
	Snippet    string `json:"snippet,omitempty"`
	FaviconURL string `json:"faviconUrl,omitempty"`
}

func presentWebSearchResult(result tool.Result) tool.Result {
	raw, ok := decodeResult[webSearchExecutionResult](result, "results")
	if !ok {
		return result
	}
	items := make([]WebSearchHit, 0, len(raw.Results))
	for _, item := range raw.Results {
		faviconURL := item.FaviconURL
		if faviconURL == "" {
			faviconURL = item.FaviconURLWire
		}
		items = append(items, WebSearchHit{
			Title: item.Title, URL: item.URL, Snippet: item.Snippet, FaviconURL: faviconURL,
		})
	}
	return projectResult(result, WebSearchResult{Results: items})
}

func patchResultContract() resultProjectionContract {
	return resultProjectionContract{
		project: presentApplyPatch, resultType: reflect.TypeFor[PatchResult](), enums: patchResultEnums,
	}
}

func patchResultEnums() map[reflect.Type][]string {
	return map[reflect.Type][]string{
		reflect.TypeFor[ChangeStatus](): changeStatusValues(),
	}
}

// PatchResult is the transcript result of apply_patch.
type PatchResult struct {
	Changes []AppliedChange `json:"changes"`
}

// ChangeStatus describes the applied filesystem mutation, not its VCS state.
type ChangeStatus string

const (
	changeAdded    ChangeStatus = "added"
	changeDeleted  ChangeStatus = "deleted"
	changeModified ChangeStatus = "modified"
	changeMoved    ChangeStatus = "moved"
)

func changeStatusValues() []string {
	return []string{string(changeAdded), string(changeDeleted), string(changeModified), string(changeMoved)}
}

// AppliedChange is one path mutation applied by apply_patch. From is present
// only when Status is moved.
type AppliedChange struct {
	Path   string       `json:"path"`
	Status ChangeStatus `json:"status"`
	From   string       `json:"from,omitempty"`
}

type patchExecutionResult struct {
	Files []struct {
		Path      string `json:"path"`
		Created   bool   `json:"created"`
		Deleted   bool   `json:"deleted"`
		MovedFrom string `json:"moved_from"`
	} `json:"files"`
}

func presentApplyPatchResult(result tool.Result) tool.Result {
	if _, ok := decodeResult[PatchResult](result, "changes"); ok {
		return result
	}
	decoded, ok := decodeResult[patchExecutionResult](result, "files")
	if !ok || len(decoded.Files) == 0 {
		return result
	}
	changes := make([]AppliedChange, 0, len(decoded.Files))
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
		changes = append(changes, AppliedChange{
			Path: file.Path, Status: status, From: file.MovedFrom,
		})
	}
	return projectResult(result, PatchResult{Changes: changes})
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
	CommandResult | SearchResult | WebSearchResult | PatchResult
}

func projectResult[T presentationValue](original tool.Result, value T) tool.Result {
	projected, err := tool.NewResult(value)
	if err != nil {
		return original
	}
	return projected
}
