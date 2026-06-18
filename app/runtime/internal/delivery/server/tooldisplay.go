package server

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// §4.4.2 display convention: server-side knowledge of how each
// built-in tool's raw output is shaped into the wire result the
// client renders. This file changes when the frontend display
// contract does.

// newToolInvocation constructs a wire ToolInvocation. For completed tools
// (outputJSON non-empty), the result is shaped per the display convention
// keyed by Name (§4.4.2: bash→{exitCode,output,…}, grep/glob→{hits}, …).
func (t *translator) newToolInvocation(name, argsJSON, outputJSON string) *protocol.ToolInvocation {
	inv := protocol.NewToolInvocation(name, argsJSON, outputJSON)
	// Apply name-based result shaping for completed tools — the display
	// convention is server-side knowledge keyed by tool name.
	if outputJSON != "" {
		inv.Result = t.shapeToolResult(name, inv.Arguments, outputJSON)
	}
	return inv
}

// shapeToolResult shapes a completed tool's result per the §4.4.2 display
// convention keyed by tool name. Unknown tools fall back to generic JSON.
func (t *translator) shapeToolResult(name string, args map[string]any, outputJSON string) any {
	switch strings.ToLower(name) {
	case "bash", "shell":
		return commandResultFrom(outputJSON)
	case "grep", "glob":
		return searchResult{Hits: parseLocalSearchHits(outputJSON)}
	case "web_search":
		return webSearchResultSet{Results: parseWebSearchHits(outputJSON)}
	case "write", "edit":
		if path := argString(args, "file_path"); path != "" {
			return fileChangeResult{Changes: []protocol.FileEdit{{Path: path, Status: "modified"}}}
		}
		return protocol.BestEffortJSON(outputJSON)
	default:
		return protocol.BestEffortJSON(outputJSON)
	}
}

// ── display-convention helpers (pure data transforms) ────────────────

// isCommandTool reports whether a tool name is a shell/command tool.
func isCommandTool(name string) bool {
	switch strings.ToLower(name) {
	case "bash", "shell":
		return true
	default:
		return false
	}
}

// ── result type projections (server-side, §4.4.2 display convention) ─

type (
	commandResult struct {
		ExitCode        *int   `json:"exitCode,omitempty"`
		Output          string `json:"output"`
		OutputTruncated bool   `json:"outputTruncated,omitempty"`
	}
	searchResult struct {
		Hits []protocol.SearchHit `json:"hits"`
	}
	webSearchResultSet struct {
		Results []protocol.WebSearchResult `json:"results"`
	}
	fileChangeResult struct {
		Changes []protocol.FileEdit `json:"changes"`
	}
)

func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func commandResultFrom(outputJSON string) commandResult {
	r := commandResult{Output: commandOutput(outputJSON)}
	var out struct {
		ExitCode *int `json:"exit_code"`
	}
	// Pointer so an absent exit_code stays nil: a command moved to the
	// background hasn't exited, so it must not render a phantom "exit 0".
	if json.Unmarshal([]byte(outputJSON), &out) == nil && out.ExitCode != nil {
		r.ExitCode = out.ExitCode
	}
	return r
}

func commandOutput(outputJSON string) string {
	var out struct {
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	_ = json.Unmarshal([]byte(outputJSON), &out)
	switch {
	case out.Stderr == "":
		return out.Stdout
	case out.Stdout == "":
		return out.Stderr
	default:
		return out.Stdout + "\n" + out.Stderr
	}
}

func parseLocalSearchHits(outputJSON string) []protocol.SearchHit {
	var out struct {
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
	if err := json.Unmarshal([]byte(outputJSON), &out); err != nil {
		return nil
	}
	var hits []protocol.SearchHit
	for _, m := range out.Matches {
		hits = append(hits, protocol.SearchHit{Path: m.Path, LineNumber: m.Line, Snippet: m.Text})
	}
	for _, p := range append(out.Files, out.Paths...) {
		hits = append(hits, protocol.SearchHit{Path: p})
	}
	for _, c := range out.Counts {
		hits = append(hits, protocol.SearchHit{Path: c.Path, Snippet: strconv.Itoa(c.Count) + " matches"})
	}
	return hits
}

func parseWebSearchHits(outputJSON string) []protocol.WebSearchResult {
	var out struct {
		Results []struct {
			Title      string `json:"title"`
			URL        string `json:"url"`
			Snippet    string `json:"snippet"`
			FaviconURL string `json:"favicon_url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &out); err != nil {
		return nil
	}
	hits := make([]protocol.WebSearchResult, 0, len(out.Results))
	for _, r := range out.Results {
		hits = append(hits, protocol.WebSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet, FaviconURL: r.FaviconURL})
	}
	return hits
}
