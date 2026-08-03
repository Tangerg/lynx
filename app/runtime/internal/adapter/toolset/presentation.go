package toolset

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

const (
	toolNameApplyPatch      = "apply_patch"
	toolNameAskUser         = "ask_user"
	toolNameEdit            = "edit"
	toolNameGlob            = "glob"
	toolNameGrep            = "grep"
	toolNameRead            = "read"
	toolNameShell           = "shell"
	toolNameReadShellOutput = "read_shell_output"
	toolNameStopShell       = "stop_shell"
	toolNameDelegateTask    = "delegate_task"
	toolNameSetPlan         = "set_plan"
	toolNameWebFetch        = "web_fetch"
	toolNameWebSearch       = "web_search"
	toolNameWrite           = "write"
)

// Presenter owns the client-facing projection of concrete tool schemas. Its
// zero value is ready for use. Unknown tools retain their canonical result and
// let the turn adapter supply generic activity text.
type Presenter struct{}

// Activity returns concise progress text for a known concrete tool.
func (Presenter) Activity(name string) string {
	switch strings.ToLower(name) {
	case toolNameShell:
		return "Running command"
	case toolNameReadShellOutput:
		return "Reading command output"
	case toolNameStopShell:
		return "Stopping command"
	case toolNameRead:
		return "Reading file"
	case toolNameWrite:
		return "Writing file"
	case toolNameEdit:
		return "Editing file"
	case toolNameApplyPatch:
		return "Applying a patch"
	case toolNameGrep:
		return "Searching"
	case toolNameGlob:
		return "Finding files"
	case toolNameWebSearch:
		return "Searching the web"
	case toolNameWebFetch:
		return "Fetching a page"
	case toolNameDelegateTask:
		return "Delegating to a sub-agent"
	case toolNameAskUser:
		return "Waiting for your answer"
	case toolNameSetPlan:
		return "Updating the plan"
	default:
		return ""
	}
}

// Present projects a known tool's canonical arguments and result into the
// client transcript shape. The second result is optional plain output for
// clients that render command text separately.
func (Presenter) Present(name string, arguments tool.Arguments, result tool.Result) (tool.Result, string) {
	switch strings.ToLower(name) {
	case toolNameShell:
		return presentCommandResult(result)
	case toolNameEdit:
		return presentEditResult(arguments, result), ""
	case toolNameWrite:
		return presentWriteResult(arguments, result), ""
	case toolNameApplyPatch:
		return presentApplyPatchResult(result), ""
	case toolNameGrep, toolNameGlob:
		return presentSearchResult(result), ""
	case toolNameWebSearch:
		return presentWebSearchResult(result), ""
	default:
		return result, ""
	}
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

type editArguments struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type writeArguments struct {
	FilePath string `json:"file_path"`
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
	Diff   []diffRow    `json:"diff,omitempty"`
}

type diffRowType string

const (
	diffRowAdded   diffRowType = "added"
	diffRowContext diffRowType = "context"
	diffRowDeleted diffRowType = "deleted"
)

type diffRow struct {
	Type      diffRowType `json:"type"`
	LeftLine  int         `json:"leftLine,omitempty"`
	RightLine int         `json:"rightLine,omitempty"`
	Code      string      `json:"code"`
}

func presentEditResult(arguments tool.Arguments, result tool.Result) tool.Result {
	if _, ok := decodeResult[changePresentation](result, "changes"); ok {
		return result
	}
	args, ok := decodeArguments[editArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	change := presentedChange{Path: args.FilePath, Status: changeModified}
	change.Diff = editDiff(args.OldString, args.NewString)
	return projectResult(result, changePresentation{Changes: []presentedChange{change}})
}

type applyPatchResult struct {
	Files []struct {
		FilePath  string `json:"file_path"`
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
			Path: file.FilePath, Status: status, From: file.MovedFrom,
		})
	}
	return projectResult(result, changePresentation{Changes: changes})
}

func presentWriteResult(arguments tool.Arguments, result tool.Result) tool.Result {
	if _, ok := decodeResult[changePresentation](result, "changes"); ok {
		return result
	}
	args, ok := decodeArguments[writeArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	return projectResult(result, changePresentation{Changes: []presentedChange{{
		Path: args.FilePath, Status: changeModified,
	}}})
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

func editDiff(oldText, newText string) []diffRow {
	if oldText == newText {
		return nil
	}
	oldLines, newLines := splitPresentationLines(oldText), splitPresentationLines(newText)
	matcher := difflib.NewMatcher(oldLines, newLines)
	rows := []diffRow{}
	left, right := 1, 1
	for _, operation := range matcher.GetOpCodes() {
		switch operation.Tag {
		case 'e':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, diffRow{Type: diffRowContext, LeftLine: left, RightLine: right, Code: oldLines[i]})
				left++
				right++
			}
		case 'd', 'r':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, diffRow{Type: diffRowDeleted, LeftLine: left, Code: oldLines[i]})
				left++
			}
			if operation.Tag != 'r' {
				continue
			}
			fallthrough
		case 'i':
			for i := operation.J1; i < operation.J2; i++ {
				rows = append(rows, diffRow{Type: diffRowAdded, RightLine: right, Code: newLines[i]})
				right++
			}
		}
	}
	return rows
}

func splitPresentationLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}
