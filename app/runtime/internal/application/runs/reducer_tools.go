package runs

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func (r *reducer) newToolInvocation(name, argumentsJSON, outputJSON string) *ToolInvocation {
	arguments := parseArgs(argumentsJSON)
	invocation := &ToolInvocation{Name: name, Arguments: arguments}
	if outputJSON != "" {
		invocation.Result = shapeToolResult(name, arguments, outputJSON)
	}
	return invocation
}

func parseArgs(raw string) map[string]any {
	arguments := map[string]any{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &arguments)
	}
	return arguments
}

func bestEffortJSON(raw string) any {
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}

func shapeToolResult(name string, arguments map[string]any, outputJSON string) *transcript.ToolResult {
	switch strings.ToLower(name) {
	case "shell":
		result := commandResultFrom(outputJSON)
		return &transcript.ToolResult{Kind: transcript.CommandToolResult, Command: &result}
	case "grep", "glob":
		result := SearchResult{Hits: parseLocalSearchHits(outputJSON)}
		return &transcript.ToolResult{Kind: transcript.SearchToolResult, Search: &result}
	case "web_search":
		result := WebSearchResultSet{Results: parseWebSearchHits(outputJSON)}
		return &transcript.ToolResult{Kind: transcript.WebSearchToolResult, WebSearch: &result}
	case "edit":
		if path := argString(arguments, "file_path"); path != "" {
			result := FileChangeResult{Changes: []FileEdit{{
				Path: path, Status: "modified",
				Diff: editDiffRows(argString(arguments, "old_string"), argString(arguments, "new_string")),
			}}}
			return &transcript.ToolResult{Kind: transcript.FileChangeToolResult, FileChange: &result}
		}
	case "write":
		if path := argString(arguments, "file_path"); path != "" {
			result := FileChangeResult{Changes: []FileEdit{{Path: path, Status: "modified"}}}
			return &transcript.ToolResult{Kind: transcript.FileChangeToolResult, FileChange: &result}
		}
	}
	return &transcript.ToolResult{Kind: transcript.RawToolResult, Raw: bestEffortJSON(outputJSON)}
}

func isCommandTool(name string) bool { return strings.EqualFold(name, "shell") }

func argString(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func commandResultFrom(outputJSON string) CommandResult {
	result := CommandResult{Output: commandOutput(outputJSON)}
	var output struct {
		ExitCode *int `json:"exit_code"`
	}
	if json.Unmarshal([]byte(outputJSON), &output) == nil {
		result.ExitCode = output.ExitCode
	}
	return result
}

func commandOutput(outputJSON string) string {
	var output struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	_ = json.Unmarshal([]byte(outputJSON), &output)
	switch {
	case output.Stderr == "":
		return output.Stdout
	case output.Stdout == "":
		return output.Stderr
	default:
		return output.Stdout + "\n" + output.Stderr
	}
}

func parseLocalSearchHits(outputJSON string) []SearchHit {
	var output struct {
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
		Files  []string `json:"files"`
		Paths  []string `json:"paths"`
		Counts []struct {
			Path  string `json:"path"`
			Count int    `json:"count"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		return nil
	}
	var hits []SearchHit
	for _, match := range output.Matches {
		hits = append(hits, SearchHit{Path: match.Path, LineNumber: match.Line, Snippet: match.Text})
	}
	for _, path := range append(output.Files, output.Paths...) {
		hits = append(hits, SearchHit{Path: path})
	}
	for _, count := range output.Counts {
		hits = append(hits, SearchHit{Path: count.Path, Snippet: strconv.Itoa(count.Count) + " matches"})
	}
	return hits
}

func parseWebSearchHits(outputJSON string) []WebSearchResult {
	var output struct {
		Results []struct {
			Title      string `json:"title"`
			URL        string `json:"url"`
			Snippet    string `json:"snippet"`
			FaviconURL string `json:"favicon_url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		return nil
	}
	results := make([]WebSearchResult, len(output.Results))
	for i, result := range output.Results {
		results[i] = WebSearchResult{
			Title: result.Title, URL: result.URL,
			Snippet: result.Snippet, FaviconURL: result.FaviconURL,
		}
	}
	return results
}

func editDiffRows(oldText, newText string) []DiffRow {
	if oldText == newText {
		return nil
	}
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	matcher := difflib.NewMatcher(oldLines, newLines)
	var rows []DiffRow
	left, right := 1, 1
	emitDeletes := func(start, end int) {
		for i := start; i < end; i++ {
			rows = append(rows, DiffRow{Kind: DiffDeleted, LeftLine: left, Code: oldLines[i]})
			left++
		}
	}
	emitAdds := func(start, end int) {
		for i := start; i < end; i++ {
			rows = append(rows, DiffRow{Kind: DiffAdded, RightLine: right, Code: newLines[i]})
			right++
		}
	}
	for _, operation := range matcher.GetOpCodes() {
		switch operation.Tag {
		case 'e':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, DiffRow{Kind: DiffContext, LeftLine: left, RightLine: right, Code: oldLines[i]})
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
