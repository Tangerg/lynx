package server

import (
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Run-terminal shaping: how a finished turn becomes the wire
// RunOutcome — outcome kind, usage roll-up, and error classification.
// This file changes when the outcome contract does.

func (t *translator) outcome(e turn.TurnEnd) *protocol.RunOutcome {
	res := &protocol.RunResult{Usage: t.turnUsage(e)}
	switch e.Reason {
	case turn.TurnEndCanceled:
		return &protocol.RunOutcome{Type: protocol.OutcomeCanceled, Result: res}
	case turn.TurnEndBudgetExceeded:
		return &protocol.RunOutcome{Type: protocol.OutcomeMaxBudget, Result: res}
	case turn.TurnEndErrored:
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
		return &protocol.ProblemData{Type: "provider_error", Channel: protocol.ErrorChannelRun, Detail: detail}
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
func (t *translator) turnUsage(e turn.TurnEnd) *protocol.Usage {
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

// optCostUSD returns &c only when c > 0, else nil (API.md §4.2).
func optCostUSD(c float64) *float64 {
	if c > 0 {
		return &c
	}
	return nil
}
