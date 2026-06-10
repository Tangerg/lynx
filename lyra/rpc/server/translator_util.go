package server

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// drainedToolsFrom captures every running tool item currently tracked
// in tools as [interrupts.DrainedTool] records. Called before
// [drainTools] so the pending interrupt can carry their
// (name, args, itemID) for resume reuse.
func drainedToolsFrom(tools map[string]*openTool) []interrupts.DrainedTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]interrupts.DrainedTool, 0, len(tools))
	for _, ref := range tools {
		out = append(out, interrupts.DrainedTool{
			ItemID:    ref.id,
			Name:      ref.name,
			Arguments: ref.args,
		})
	}
	return out
}

func (t *translator) drainTools() []protocol.StreamEvent {
	if len(t.tools) == 0 {
		return nil
	}
	out := make([]protocol.StreamEvent, 0, len(t.tools))
	for callID, ref := range t.tools {
		out = append(out, protocol.StreamEvent{
			Type: protocol.StreamItemCompleted,
			Item: &protocol.Item{
				ID:        ref.id,
				RunID:     ref.runID,
				Status:    protocol.ItemStatusIncomplete,
				Type:      protocol.ItemTypeToolCall,
				CreatedAt: ref.createdAt,
				Tool:      t.newToolInvocation(ref.name, ref.args, ""),
			},
		})
		delete(t.tools, callID)
	}
	return out
}

func (t *translator) outcome(e chat.TurnEnd) *protocol.RunOutcome {
	res := &protocol.RunResult{Usage: t.turnUsage(e)}
	switch e.Reason {
	case chat.TurnEndCancelled:
		return &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Result: res}
	case chat.TurnEndBudgetExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxBudget, Result: res}
	case chat.TurnEndErrored:
		res.Error = t.classifyRunError(t.errMsg)
		return &protocol.RunOutcome{Type: protocol.OutcomeError, Result: res}
	default:
		return &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: res}
	}
}

// classifyRunError maps a failed run's error message onto a wire
// ProblemData, classifying by provider-visible patterns.
func (t *translator) classifyRunError(msg string) *protocol.ProblemData {
	m := strings.ToLower(msg)
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	provider := func(detail string) *protocol.ProblemData {
		return &protocol.ProblemData{Type: "provider_error", Channel: "run", Detail: detail}
	}
	switch {
	case contains("429", "too many requests", "rate limit", "overloaded", "quota"):
		return provider("the model provider rate-limited the request; retry shortly")
	case contains(" 401", " 403", "unauthorized", "forbidden", "invalid_api_key", "api key"):
		return provider("the model provider rejected the credentials; check the provider API key")
	case contains(" 500", " 502", " 503", " 504", "bad gateway", "service unavailable", "internal server error"):
		return provider("the model provider is temporarily unavailable; retry shortly")
	case contains("deadline exceeded", "timeout", "timed out", "client.timeout", "connection refused", "no such host", "i/o timeout", "eof", "connection reset"):
		return provider("the model provider request timed out or the connection failed; retry shortly")
	case contains(" 400", "invalid_request_error", "bad request"):
		return provider("the model provider rejected the request as invalid")
	default:
		return protocol.InternalErrorProblem()
	}
}

// turnUsage maps the engine's per-turn token roll-up onto wire Usage.
func (t *translator) turnUsage(e chat.TurnEnd) *protocol.Usage {
	u := &protocol.Usage{
		ModelUsage: protocol.ModelUsage{
			InputTokens:     e.TokenUsage.PromptTokens,
			OutputTokens:    e.TokenUsage.CompletionTokens,
			ReasoningTokens: e.TokenUsage.ReasoningTokens,
			CostUSD:         optCostUSD(e.CostUSD),
		},
	}
	if len(e.UsageByModel) > 0 {
		u.ByModel = make(map[string]protocol.ModelUsage, len(e.UsageByModel))
		for _, m := range e.UsageByModel {
			u.ByModel[m.Model] = protocol.ModelUsage{
				InputTokens:  m.PromptTokens,
				OutputTokens: m.CompletionTokens,
				CostUSD:      optCostUSD(m.CostUSD),
			}
		}
	}
	return u
}

// newToolInvocation constructs a wire ToolInvocation. For completed tools
// (outputJSON non-empty), the result is shaped per the display convention
// keyed by Name (§4.4.2: bash→{exitCode,output,…}, grep/glob→{hits}, …).
func (t *translator) newToolInvocation(name, argsJSON, outputJSON string) *protocol.ToolInvocation {
	inv := protocol.NewToolInvocation(name, argsJSON, outputJSON)
	// Apply name-based result shaping for completed tools — the display
	// convention is server-side knowledge keyed by tool name.
	if outputJSON != "" && inv.Arguments != nil {
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
	case "websearch":
		return webSearchResultSet{Results: parseWebSearchHits(outputJSON)}
	case "write", "edit":
		if path := argString(args, "path"); path != "" {
			return fileChangeResult{Changes: []protocol.FileEdit{{Path: path, Status: "modified"}}}
		}
		return protocol.BestEffortJSON(outputJSON)
	default:
		return protocol.BestEffortJSON(outputJSON)
	}
}

// ── display-convention helpers (pure data transforms) ────────────────

// optCostUSD returns &c only when c > 0, else nil (API.md §4.2).
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}

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
		ExitCode int `json:"exit_code"`
	}
	if json.Unmarshal([]byte(outputJSON), &out) == nil {
		ec := out.ExitCode
		r.ExitCode = &ec
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
