// Package toolpresentation projects canonical tool records into the stable
// client-facing JSON shapes. It is deliberately independent of Delivery: the
// protocol adapter asks for a projection but never knows concrete tool names,
// vendor result fields, or diff construction rules.
package toolpresentation

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type resultPresenter func(arguments map[string]any, result any) any

type presentation struct {
	activity string
	result   resultPresenter
}

var presentations = map[string]presentation{
	"shell":             {activity: "Running command", result: presentCommandResult},
	"run_in_background": {activity: "Running command"},
	"shell_output":      {activity: "Reading command output"},
	"shell_kill":        {activity: "Stopping command"},
	"read":              {activity: "Reading file"},
	"write":             {activity: "Writing file", result: presentWriteResult},
	"edit":              {activity: "Editing file", result: presentEditResult},
	"grep":              {activity: "Searching", result: presentSearchResult},
	"glob":              {activity: "Finding files", result: presentSearchResult},
	"web_search":        {activity: "Searching the web", result: presentWebSearchResult},
	"web_fetch":         {activity: "Fetching a page"},
	"task":              {activity: "Delegating to a sub-agent"},
	"subagent":          {activity: "Delegating to a sub-agent"},
	"ask_user":          {activity: "Waiting for your answer"},
	"todo_write":        {activity: "Updating the plan"},
}

// Present returns the stable JSON result shape for a known tool. Unknown tools
// and already-projected results pass through unchanged, making extension a
// registration concern rather than a Delivery conditional.
func Present(name string, arguments map[string]any, result any) any {
	p := presentations[strings.ToLower(name)]
	if p.result == nil {
		return result
	}
	return p.result(arguments, result)
}

// Activity supplies the human-readable live activity for a tool call.
func Activity(name string) string {
	if name == "" {
		return ""
	}
	if activity := presentations[strings.ToLower(name)].activity; activity != "" {
		return activity
	}
	return "Calling " + name
}

type commandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode *int   `json:"exit_code"`
}

func presentCommandResult(_ map[string]any, result any) any {
	if hasField(result, "output") {
		return result
	}
	raw, ok := decode[commandResult](result, "stdout", "stderr", "exit_code")
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
	out := map[string]any{"output": output}
	if raw.ExitCode != nil {
		out["exitCode"] = *raw.ExitCode
	}
	return out
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

func presentSearchResult(_ map[string]any, result any) any {
	if hasField(result, "hits") {
		return result
	}
	raw, ok := decode[localSearchResult](result, "matches", "files", "paths", "counts")
	if !ok {
		return result
	}
	hits := make([]map[string]any, 0, len(raw.Matches)+len(raw.Files)+len(raw.Paths)+len(raw.Counts))
	for _, match := range raw.Matches {
		hit := map[string]any{"path": match.Path, "snippet": match.Text}
		if match.Line != 0 {
			hit["lineNumber"] = match.Line
		}
		hits = append(hits, hit)
	}
	for _, path := range raw.Files {
		hits = append(hits, map[string]any{"path": path})
	}
	for _, path := range raw.Paths {
		hits = append(hits, map[string]any{"path": path})
	}
	for _, count := range raw.Counts {
		hits = append(hits, map[string]any{"path": count.Path, "snippet": strconv.Itoa(count.Count) + " matches"})
	}
	return map[string]any{"hits": hits}
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

func presentWebSearchResult(_ map[string]any, result any) any {
	raw, ok := decode[webSearchResult](result, "results")
	if !ok {
		return result
	}
	items := make([]map[string]any, 0, len(raw.Results))
	for _, item := range raw.Results {
		out := map[string]any{"url": item.URL}
		if item.Title != "" {
			out["title"] = item.Title
		}
		if item.Snippet != "" {
			out["snippet"] = item.Snippet
		}
		if item.FaviconURL != "" {
			out["faviconUrl"] = item.FaviconURL
		} else if item.FaviconURLWire != "" {
			out["faviconUrl"] = item.FaviconURLWire
		}
		items = append(items, out)
	}
	return map[string]any{"results": items}
}

type editArguments struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type writeArguments struct {
	FilePath string `json:"file_path"`
}

func presentEditResult(arguments map[string]any, result any) any {
	if hasField(result, "changes") {
		return result
	}
	args, ok := decode[editArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	change := map[string]any{"path": args.FilePath, "status": "modified"}
	if diff := editDiff(args.OldString, args.NewString); len(diff) != 0 {
		change["diff"] = diff
	}
	return map[string]any{"changes": []map[string]any{change}}
}

func presentWriteResult(arguments map[string]any, result any) any {
	if hasField(result, "changes") {
		return result
	}
	args, ok := decode[writeArguments](arguments, "file_path")
	if !ok || args.FilePath == "" {
		return result
	}
	return map[string]any{"changes": []map[string]any{{"path": args.FilePath, "status": "modified"}}}
}

func decode[T any](value any, knownFields ...string) (T, bool) {
	var decoded T
	data, err := json.Marshal(value)
	if err != nil {
		return decoded, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return decoded, false
	}
	for _, field := range knownFields {
		if _, ok := fields[field]; ok {
			if json.Unmarshal(data, &decoded) == nil {
				return decoded, true
			}
			return decoded, false
		}
	}
	return decoded, false
}

func hasField(value any, field string) bool {
	_, ok := decode[struct{}](value, field)
	return ok
}

func editDiff(oldText, newText string) []map[string]any {
	if oldText == newText {
		return nil
	}
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)
	matcher := difflib.NewMatcher(oldLines, newLines)
	rows := []map[string]any{}
	left, right := 1, 1
	for _, operation := range matcher.GetOpCodes() {
		switch operation.Tag {
		case 'e':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, map[string]any{"type": "context", "leftLine": left, "rightLine": right, "code": oldLines[i]})
				left++
				right++
			}
		case 'd', 'r':
			for i := operation.I1; i < operation.I2; i++ {
				rows = append(rows, map[string]any{"type": "deleted", "leftLine": left, "code": oldLines[i]})
				left++
			}
			if operation.Tag != 'r' {
				continue
			}
			fallthrough
		case 'i':
			for i := operation.J1; i < operation.J2; i++ {
				rows = append(rows, map[string]any{"type": "added", "rightLine": right, "code": newLines[i]})
				right++
			}
		}
	}
	return rows
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}
