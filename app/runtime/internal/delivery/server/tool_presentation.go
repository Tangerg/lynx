package server

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type toolResultPresenter func(arguments map[string]any, result any) any

type toolPresentation struct {
	activity      string
	presentResult toolResultPresenter
}

type commandToolResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode *int   `json:"exit_code"`
}

type commandResultPresentation struct {
	ExitCode *int   `json:"exitCode,omitempty"`
	Output   string `json:"output"`
}

type localSearchToolResult struct {
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

type searchResultPresentation struct {
	Hits []protocol.SearchHit `json:"hits"`
}

type webSearchToolResult struct {
	Results []webSearchHit `json:"results"`
}

type webSearchHit struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	Snippet        string `json:"snippet"`
	FaviconURL     string `json:"favicon_url"`
	FaviconURLWire string `json:"faviconUrl"`
}

func (h webSearchHit) faviconURL() string {
	if h.FaviconURL != "" {
		return h.FaviconURL
	}
	return h.FaviconURLWire
}

type webSearchResultPresentation struct {
	Results []protocol.WebSearchResult `json:"results"`
}

type editToolArguments struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type writeToolArguments struct {
	FilePath string `json:"file_path"`
}

type fileChangesPresentation struct {
	Changes []protocol.FileEdit `json:"changes"`
}

var toolPresentations = map[string]toolPresentation{
	"shell":             {activity: "Running command", presentResult: presentCommandResult},
	"run_in_background": {activity: "Running command"},
	"shell_output":      {activity: "Reading command output"},
	"shell_kill":        {activity: "Stopping command"},
	"read":              {activity: "Reading file"},
	"write":             {activity: "Writing file", presentResult: presentWriteResult},
	"edit":              {activity: "Editing file", presentResult: presentEditResult},
	"grep":              {activity: "Searching", presentResult: presentSearchResult},
	"glob":              {activity: "Finding files", presentResult: presentSearchResult},
	"web_search":        {activity: "Searching the web", presentResult: presentWebSearchResult},
	"web_fetch":         {activity: "Fetching a page"},
	"task":              {activity: "Delegating to a sub-agent"},
	"subagent":          {activity: "Delegating to a sub-agent"},
	"ask_user":          {activity: "Waiting for your answer"},
	"todo_write":        {activity: "Updating the plan"},
}

func presentToolResult(tool transcript.ToolInvocation) any {
	if tool.Result == nil {
		return nil
	}
	presentation := toolPresentations[strings.ToLower(tool.Name)]
	if presentation.presentResult == nil {
		return tool.Result
	}
	return presentation.presentResult(tool.Arguments, tool.Result)
}

func presentCommandResult(_ map[string]any, result any) any {
	if hasToolField(result, "output") {
		return result
	}
	raw, ok := decodeToolValue[commandToolResult](result, "stdout", "stderr", "exit_code")
	if !ok {
		return result
	}
	output := raw.Stdout
	switch {
	case raw.Stdout == "":
		output = raw.Stderr
	case raw.Stderr != "":
		output = raw.Stdout + "\n" + raw.Stderr
	}
	return commandResultPresentation{
		ExitCode: raw.ExitCode,
		Output:   output,
	}
}

func presentSearchResult(_ map[string]any, result any) any {
	if hasToolField(result, "hits") {
		return result
	}
	raw, ok := decodeToolValue[localSearchToolResult](result, "matches", "files", "paths", "counts")
	if !ok {
		return result
	}
	hits := make([]protocol.SearchHit, 0, len(raw.Matches)+len(raw.Files)+len(raw.Paths)+len(raw.Counts))
	for _, match := range raw.Matches {
		hits = append(hits, protocol.SearchHit{
			Path:       match.Path,
			LineNumber: match.Line,
			Snippet:    match.Text,
		})
	}
	for _, path := range raw.Files {
		hits = append(hits, protocol.SearchHit{Path: path})
	}
	for _, path := range raw.Paths {
		hits = append(hits, protocol.SearchHit{Path: path})
	}
	for _, count := range raw.Counts {
		hits = append(hits, protocol.SearchHit{
			Path:    count.Path,
			Snippet: strconv.Itoa(count.Count) + " matches",
		})
	}
	return searchResultPresentation{Hits: hits}
}

func presentWebSearchResult(_ map[string]any, result any) any {
	raw, ok := decodeToolValue[webSearchToolResult](result, "results")
	if !ok {
		return result
	}
	results := make([]protocol.WebSearchResult, 0, len(raw.Results))
	for _, item := range raw.Results {
		results = append(results, protocol.WebSearchResult{
			Title:      item.Title,
			URL:        item.URL,
			Snippet:    item.Snippet,
			FaviconURL: item.faviconURL(),
		})
	}
	return webSearchResultPresentation{Results: results}
}

func presentEditResult(arguments map[string]any, result any) any {
	if hasToolField(result, "changes") {
		return result
	}
	args, ok := decodeToolValue[editToolArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	return fileChangesPresentation{Changes: []protocol.FileEdit{{
		Path: args.FilePath, Status: protocol.FileStatusModified,
		Diff: editDiffRows(
			args.OldString,
			args.NewString,
		),
	}}}
}

func presentWriteResult(arguments map[string]any, result any) any {
	if hasToolField(result, "changes") {
		return result
	}
	args, ok := decodeToolValue[writeToolArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	return fileChangesPresentation{Changes: []protocol.FileEdit{{
		Path: args.FilePath, Status: protocol.FileStatusModified,
	}}}
}

func decodeToolValue[T any](value any, knownFields ...string) (T, bool) {
	var decoded T
	data, err := json.Marshal(value)
	if err != nil {
		return decoded, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return decoded, false
	}
	known := false
	for _, field := range knownFields {
		if _, ok := fields[field]; ok {
			known = true
			break
		}
	}
	if !known {
		return decoded, false
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return decoded, false
	}
	return decoded, true
}

func hasToolField(value any, field string) bool {
	_, ok := decodeToolValue[struct{}](value, field)
	return ok
}

func editDiffRows(oldText, newText string) []protocol.DiffRow {
	if oldText == newText {
		return nil
	}
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	matcher := difflib.NewMatcher(oldLines, newLines)
	var rows []protocol.DiffRow
	left, right := 1, 1
	emitDeletes := func(start, end int) {
		for i := start; i < end; i++ {
			rows = append(rows, protocol.DiffRow{
				Type: protocol.DiffRowDeleted, LeftLine: left, Code: oldLines[i],
			})
			left++
		}
	}
	emitAdds := func(start, end int) {
		for i := start; i < end; i++ {
			rows = append(rows, protocol.DiffRow{
				Type: protocol.DiffRowAdded, RightLine: right, Code: newLines[i],
			})
			right++
		}
	}
	for _, operation := range matcher.GetOpCodes() {
		switch operation.Tag {
		case 'e':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, protocol.DiffRow{
					Type: protocol.DiffRowContext, LeftLine: left,
					RightLine: right, Code: oldLines[i],
				})
				left++
				right++
			}
		case 'd':
			emitDeletes(operation.I1, operation.I2)
		case 'i':
			emitAdds(operation.J1, operation.J2)
		case 'r':
			emitDeletes(operation.I1, operation.I2)
			emitAdds(operation.J1, operation.J2)
		}
	}
	return rows
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func toolActivity(name string) string {
	if name == "" {
		return ""
	}
	if activity := toolPresentations[strings.ToLower(name)].activity; activity != "" {
		return activity
	}
	return "Calling " + name
}
