package server

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// internalErrorProblem builds the wire ProblemData for a run that failed
// with an internal error. The detail is a clean, generic message — the full
// error (with any wrapped Go context) rides the server-side turn span, never
// the wire (API.md §8.2: detail is a user/agent-readable note, not an
// implementation call path). After tool failures stopped escalating to run
// errors (FeedbackOnToolError), this path is genuine engine/infra failure.
func internalErrorProblem() *protocol.ProblemData {
	return &protocol.ProblemData{Type: "internal_error", Channel: "run", Detail: "the run failed due to an internal error"}
}

// classifyRunError maps a failed run's (server-side, full) error message
// onto a wire ProblemData. Errors that originate at the model provider —
// rate limits, provider 5xx, auth/bad-request, timeouts/network — surface
// as a distinct provider_error (API.md §8.2, code -32001) with an
// actionable but NON-leaking detail (a fixed per-class string, never the
// raw message / URL / Go call path), so the client can react (back off on a
// rate limit, prompt for a key on auth) instead of treating every transient
// blip as an opaque internal_error and hammer-retrying. Anything
// unrecognized stays internal_error. The raw message still rides only the
// server-side span.
func classifyRunError(msg string) *protocol.ProblemData {
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
		return internalErrorProblem()
	}
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
				Tool:      toolInvocation(ref.name, ref.args, ""),
			},
		})
		delete(t.tools, callID)
	}
	return out
}

func (t *translator) outcome(e chat.TurnEnd) *protocol.RunOutcome {
	res := &protocol.RunResult{Usage: turnUsage(e)}
	switch e.Reason {
	case chat.TurnEndCancelled:
		return &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Result: res}
	case chat.TurnEndBudgetExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxBudget, Result: res}
	case chat.TurnEndErrored:
		res.Error = classifyRunError(t.errMsg)
		return &protocol.RunOutcome{Type: protocol.OutcomeError, Result: res}
	default:
		return &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: res}
	}
}

// turnUsage maps the engine's per-turn token roll-up onto wire Usage.
func turnUsage(e chat.TurnEnd) *protocol.Usage {
	// Total cost rides Usage.CostUSD (the embedded ModelUsage), not a separate
	// RunResult.costUsd (§4.2 / N1 — one source of total cost).
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

// optCostUSD returns &c only when a pricing hook produced a real figure
// (c > 0), else nil — API.md §4.2 omits cost rather than faking 0.
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}

// isCommandTool reports whether a tool name is a shell/command tool, whose
// stdout streams as a toolOutput preview and whose result is the command
// shape {exitCode, output, outputTruncated?} (§4.4.2). The name is the sole
// identity now (no kind on the wire, §4.4) — the display convention keys off
// it. Everything else falls through to a generic best-effort JSON result.
func isCommandTool(name string) bool {
	switch strings.ToLower(name) {
	case "bash", "shell":
		return true
	default:
		return false
	}
}

// toolInvocation builds the domain-neutral wire ToolInvocation for a tool
// call (API.md §4.4): Name (identity) + Arguments (parsed object, always
// present) + a best-effort JSON Result. Result is shaped per the §4.4.2
// display convention keyed by Name (bash→{exitCode,output,…}, grep/glob→
// {hits}, webSearch→{results}, edit/write→{changes}); any other tool gets a
// generic best-effort JSON result. argsJSON is the model's raw JSON
// arguments; outputJSON is the tool's JSON result ("" before completion, so
// the started shell carries no result). Tool failure / the streaming output
// preview are handled by the caller (toolEnd error mapping, toolOutput delta).
func toolInvocation(name, argsJSON, outputJSON string) *protocol.ToolInvocation {
	args := parseArgs(argsJSON)
	if args == nil {
		args = map[string]any{} // arguments is ALWAYS an object on the wire (§4.4.1)
	}
	inv := &protocol.ToolInvocation{Name: name, Arguments: args}
	if outputJSON != "" {
		inv.Result = toolResult(name, args, outputJSON)
	}
	return inv
}

// toolResult shapes a completed tool's best-effort JSON result per the
// §4.4.2 display convention keyed by tool name. The convention is
// non-normative (the client's display registry reads it) — an unknown tool
// falls back to a generic JSON value, which the client renders as a JSON
// tree. Failure never lands here (it rides toolCall.error, §4.3).
func toolResult(name string, args map[string]any, outputJSON string) any {
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
		return bestEffortJSON(outputJSON)
	default:
		return bestEffortJSON(outputJSON)
	}
}

// Result envelopes for the §4.4.2 display convention. Typed (not ad-hoc
// maps) so the shape is compile-time exact and marshals to the documented
// keys. They are server-side projection helpers, not protocol types — the
// reusable members (SearchHit / WebSearchResult / FileEdit) are.
type (
	// commandResult is the bash/shell convention: merged stdout+stderr as the
	// AUTHORITATIVE `output` (§5.2 — the toolOutput delta is only its preview)
	// + exitCode; outputTruncated flags a size-capped output. output is always
	// present (even ""), so history replay / reconnect (no delta) still renders
	// the terminal output rather than "(no output)".
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

// argString reads a string field from the parsed arguments, "" when absent.
func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// bestEffortJSON decodes raw as JSON (object / array / scalar) for a generic
// tool's result; when raw isn't valid JSON it's surfaced verbatim as a string
// (API.md §4.4: result is best-effort JSON, never double-encoded).
func bestEffortJSON(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return v
}

// commandResultFrom builds the command result from a bash tool's JSON output
// ({stdout, stderr, exit_code}): the AUTHORITATIVE merged `output` + exitCode
// (API.md §4.4.2 / §5.2). Output is always set — even "" — so a no-output
// command renders an empty terminal, not a stale preview.
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

// commandOutput merges a bash tool's stdout+stderr into the single full-text
// `output` value (API.md §4.4.2). The wire field is one stream (terminals
// interleave the two); lacking true interleave order we append stderr after
// stdout, separated by a newline when both are present.
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

// parseLocalSearchHits maps grep / glob JSON output onto SearchHit values
// (a search tool's result {hits}, §4.4.2). grep "content" mode →
// {matches:[{path,line,text}]}; grep "files_with_matches" / glob →
// {files|paths:[…]}; counts mode → {counts}.
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

// parseWebSearchHits maps websearch JSON output ({results:[{title,url,
// snippet,favicon_url}]}) onto WebSearchResult values (a webSearch tool's
// result {results}, §4.4.2).
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
