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
	raw, _ := result.(map[string]any)
	if _, presented := raw["output"]; presented {
		return result
	}
	stdout := stringField(raw, "stdout")
	stderr := stringField(raw, "stderr")
	output := stdout
	switch {
	case stdout == "":
		output = stderr
	case stderr != "":
		output = stdout + "\n" + stderr
	}
	return struct {
		ExitCode        *int   `json:"exitCode,omitempty"`
		Output          string `json:"output"`
		OutputTruncated bool   `json:"outputTruncated,omitempty"`
	}{
		ExitCode: intField(raw, "exit_code"),
		Output:   output,
	}
}

func presentSearchResult(_ map[string]any, result any) any {
	raw, _ := result.(map[string]any)
	if _, presented := raw["hits"]; presented {
		return result
	}
	var hits []protocol.SearchHit
	for _, value := range sliceField(raw, "matches") {
		match, _ := value.(map[string]any)
		hits = append(hits, protocol.SearchHit{
			Path:       stringField(match, "path"),
			LineNumber: intValue(match["line"]),
			Snippet:    stringField(match, "text"),
		})
	}
	for _, key := range []string{"files", "paths"} {
		for _, value := range sliceField(raw, key) {
			if path, ok := value.(string); ok {
				hits = append(hits, protocol.SearchHit{Path: path})
			}
		}
	}
	for _, value := range sliceField(raw, "counts") {
		count, _ := value.(map[string]any)
		hits = append(hits, protocol.SearchHit{
			Path:    stringField(count, "path"),
			Snippet: strconv.Itoa(intValue(count["count"])) + " matches",
		})
	}
	return struct {
		Hits []protocol.SearchHit `json:"hits"`
	}{Hits: hits}
}

func presentWebSearchResult(_ map[string]any, result any) any {
	raw, _ := result.(map[string]any)
	values := sliceField(raw, "results")
	results := make([]protocol.WebSearchResult, 0, len(values))
	for _, value := range values {
		item, _ := value.(map[string]any)
		results = append(results, protocol.WebSearchResult{
			Title:      stringField(item, "title"),
			URL:        stringField(item, "url"),
			Snippet:    stringField(item, "snippet"),
			FaviconURL: firstStringField(item, "favicon_url", "faviconUrl"),
		})
	}
	return struct {
		Results []protocol.WebSearchResult `json:"results"`
	}{Results: results}
}

func presentEditResult(arguments map[string]any, result any) any {
	if alreadyPresented(result, "changes") {
		return result
	}
	path := stringField(arguments, "file_path")
	if path == "" {
		return result
	}
	return struct {
		Changes []protocol.FileEdit `json:"changes"`
	}{Changes: []protocol.FileEdit{{
		Path: path, Status: protocol.FileStatusModified,
		Diff: editDiffRows(
			stringField(arguments, "old_string"),
			stringField(arguments, "new_string"),
		),
	}}}
}

func presentWriteResult(arguments map[string]any, result any) any {
	if alreadyPresented(result, "changes") {
		return result
	}
	path := stringField(arguments, "file_path")
	if path == "" {
		return result
	}
	return struct {
		Changes []protocol.FileEdit `json:"changes"`
	}{Changes: []protocol.FileEdit{{
		Path: path, Status: protocol.FileStatusModified,
	}}}
}

func alreadyPresented(result any, field string) bool {
	raw, ok := result.(map[string]any)
	if !ok {
		return false
	}
	_, ok = raw[field]
	return ok
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func firstStringField(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := stringField(value, key); text != "" {
			return text
		}
	}
	return ""
}

func sliceField(value map[string]any, key string) []any {
	items, _ := value[key].([]any)
	return items
}

func intField(value map[string]any, key string) *int {
	if _, ok := value[key]; !ok {
		return nil
	}
	number := intValue(value[key])
	return &number
}

func intValue(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		number, _ := strconv.Atoi(value.String())
		return number
	default:
		return 0
	}
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
