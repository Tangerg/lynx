package server

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Run-terminal shaping: how a finished turn becomes the wire
// RunOutcome — outcome kind, usage roll-up, and error classification.
// This file changes when the outcome contract does.

func (t *translator) outcome(e turn.TurnEnd) *protocol.RunOutcome {
	// steps lands the run's final step count durably (API.md §5.2): it's the
	// authoritative home of the ephemeral run.progress.step, so a client that
	// dropped the progress deltas can still read it off the terminal. t.step is
	// the live ordinal the translator emitted on run.progress.
	steps := t.step
	res := &protocol.RunResult{Usage: t.turnUsage(e), Steps: &steps, DurationMs: int(e.Duration.Milliseconds())}
	switch e.Reason {
	case turn.TurnEndCanceled:
		return &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Result: res}
	case turn.TurnEndBudgetExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxBudget, Result: res, Detail: budgetDetail(e)}
	case turn.TurnEndStepsExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxSteps, Result: res, Detail: stepDetail(e)}
	case turn.TurnEndErrored:
		res.Error = t.classifyRunError(t.errMsg)
		return &protocol.RunOutcome{Type: protocol.OutcomeError, Result: res}
	default:
		return &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: res}
	}
}

// budgetDetail describes which configured cap a budget-exceeded run hit, for
// RunOutcome.detail (e.g. "spent $4.20 of $4.00 budget" / "reached the
// 8000-token budget"). Falls back to a generic note when neither cap is echoed.
func budgetDetail(e turn.TurnEnd) string {
	switch {
	case e.MaxCostUSD > 0:
		return fmt.Sprintf("spent $%.2f of $%.2f budget", e.CostUSD, e.MaxCostUSD)
	case e.MaxBudget > 0:
		return fmt.Sprintf("reached the %d-token budget", e.MaxBudget)
	default:
		return "reached the configured budget"
	}
}

// stepDetail describes a maxSteps-terminated run for RunOutcome.detail
// (e.g. "reached the 8-step limit"). Falls back to a generic note if the cap
// wasn't echoed.
func stepDetail(e turn.TurnEnd) string {
	if e.MaxSteps > 0 {
		return fmt.Sprintf("reached the %d-step limit", e.MaxSteps)
	}
	return "reached the configured step limit"
}

// classifyRunError maps a failed run's error message onto a wire
// ProblemData, classifying by provider-visible patterns.
func (t *translator) classifyRunError(msg string) *protocol.ProblemData {
	// A stuck agent (the loop's no-forward-progress guard tripped) is a
	// distinct, stable failure — surface it under its own wire symbol rather
	// than letting it fall through to internal_error. Keyed on the turn-layer
	// error code, not the message text, so it can't be confused with a provider
	// error that merely mentions "stuck". Other engine errors keep flowing
	// through the provider-pattern classification below (their message is
	// usually a wrapped provider failure worth a retry hint).
	if t.errCode == "AGENT_STUCK" {
		return &protocol.ProblemData{Type: protocol.ProblemAgentStuck, Channel: protocol.ErrorChannelRun, Detail: msg}
	}
	m := strings.ToLower(msg)
	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	// problem builds a run-channel ProblemData under a stable wire symbol. The
	// symbol — not the free-text detail — is what the client branches on: each
	// distinct failure mode gets its own symbol so no consumer ever has to
	// substring-match `detail` (or lean on the retryable flag) to tell two
	// failures apart.
	problem := func(symbol, detail string) *protocol.ProblemData {
		return &protocol.ProblemData{Type: symbol, Channel: protocol.ErrorChannelRun, Detail: detail}
	}
	// retryable marks a transient failure (worth retrying) and carries a
	// best-effort backoff hint parsed from the message — so the client can gate
	// / count down its retry instead of hammering.
	retryable := func(symbol, detail string) *protocol.ProblemData {
		p := problem(symbol, detail)
		p.Retryable = true
		p.RetryAfterSeconds = parseRetryAfter(msg)
		return p
	}
	switch {
	case contains("429", "too many requests", "rate limit", "overloaded", "quota"):
		return retryable(protocol.ProblemRateLimited, "the model provider rate-limited the request; retry shortly")
	case contains(" 401", " 403", "unauthorized", "forbidden", "invalid_api_key", "api key"):
		// Not retryable: resending won't help until the key is fixed.
		return problem(protocol.ProblemInvalidAPIKey, "the model provider rejected the credentials; check the provider API key")
	case contains("deadline exceeded", "timeout", "timed out", "client.timeout", "connection refused", "no such host", "i/o timeout", "eof", "connection reset"):
		return retryable(protocol.ProblemTimeout, "the model provider request timed out or the connection failed; retry shortly")
	case contains(" 500", " 502", " 503", " 504", "bad gateway", "service unavailable", "internal server error"):
		return retryable(protocol.ProblemProviderUnavailable, "the model provider is temporarily unavailable; retry shortly")
	case contains(" 400", "invalid_request_error", "bad request"):
		// Not retryable: the request itself is malformed. Distinct from the
		// RPC-level `invalid_request` (-32600, a bad JSON-RPC envelope) — this is
		// the provider rejecting the model request we sent.
		return problem(protocol.ProblemProviderRejected, "the model provider rejected the request as invalid")
	default:
		return protocol.InternalErrorProblem()
	}
}

// retryAfterRe pulls a backoff hint out of a provider error message — a
// Retry-After header echoed into the text or a "try again in N seconds" phrase.
// Most providers don't include one, so a miss (0) is the common case.
var retryAfterRe = regexp.MustCompile(`(?i)(?:retry[- ]?after|try again in)[:\s]+(\d+)`)

// parseRetryAfter returns the provider's requested backoff in whole seconds, or
// 0 when the message carries none. Capped at one hour as a sanity bound.
func parseRetryAfter(msg string) int {
	m := retryAfterRe.FindStringSubmatch(msg)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 || n > 3600 {
		return 0
	}
	return n
}

// turnUsage maps the engine's per-turn token roll-up onto wire Usage.
func (t *translator) turnUsage(e turn.TurnEnd) *protocol.Usage {
	u := &protocol.Usage{
		ModelUsage: modelUsageFrom(e.TokenUsage.PromptTokens, e.TokenUsage.CompletionTokens, e.TokenUsage.ReasoningTokens, e.CostUSD),
	}
	if len(e.UsageByModel) > 0 {
		u.ByModel = make(map[string]protocol.ModelUsage, len(e.UsageByModel))
		for _, m := range e.UsageByModel {
			// Per-model rows carry no reasoning split.
			u.ByModel[m.Model] = modelUsageFrom(m.PromptTokens, m.CompletionTokens, 0, m.CostUSD)
		}
	}
	return u
}

// modelUsageFrom builds the wire ModelUsage from a token roll-up + cost — the
// one mapping shared by the run-final usage (turnUsage) and the mid-run usage
// preview (usageProgress), so a future usage field is added in one place. cost
// folds through optCostUSD (omitted when ≤ 0).
func modelUsageFrom(prompt, completion, reasoning int64, cost float64) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens:     prompt,
		OutputTokens:    completion,
		ReasoningTokens: reasoning,
		CostUSD:         optCostUSD(cost),
	}
}

// optCostUSD returns &c only when c > 0, else nil (API.md §4.2).
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}
