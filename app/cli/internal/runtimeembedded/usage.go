package runtimeembedded

import (
	"context"
	"errors"
	"sort"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/usage"
)

type usageBinding interface {
	GetSessionUsage(context.Context, protocol.SessionUsageRequest, embedded.CallOptions) (*protocol.Usage, error)
	GetUsageSummary(context.Context, protocol.UsageSummaryRequest, embedded.CallOptions) (*protocol.UsageSummary, error)
}

var _ usage.Service = (*Runtime)(nil)

func (r *Runtime) SessionUsage(ctx context.Context, sessionID string) (usage.SessionReport, error) {
	if sessionID == "" {
		return usage.SessionReport{}, errors.New("session usage: session id is empty")
	}
	result, err := r.usage.GetSessionUsage(ctx, protocol.SessionUsageRequest{SessionID: sessionID}, r.callOptions())
	if err != nil {
		return usage.SessionReport{}, classifyError(err)
	}
	if result == nil {
		return usage.SessionReport{}, runtimeContractViolation("session usage returned nil")
	}
	report := usage.SessionReport{
		SessionID: sessionID,
		Total:     projectUsageTotals(result.ModelUsage),
		ByModel:   make([]usage.Bucket, 0, len(result.ByModel)),
	}
	keys := make([]string, 0, len(result.ByModel))
	for key := range result.ByModel {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report.ByModel = append(report.ByModel, usage.Bucket{Key: key, Totals: projectUsageTotals(result.ByModel[key])})
	}
	if err := report.Validate(); err != nil {
		return usage.SessionReport{}, runtimeContractViolation("session usage returned an invalid report: %v", err)
	}
	return report, nil
}

func (r *Runtime) Summary(ctx context.Context, sinceDays int) (usage.Summary, error) {
	if sinceDays < 0 {
		return usage.Summary{}, errors.New("usage summary: since days is negative")
	}
	result, err := r.usage.GetUsageSummary(ctx, protocol.UsageSummaryRequest{SinceDays: sinceDays}, r.callOptions())
	if err != nil {
		return usage.Summary{}, classifyError(err)
	}
	if result == nil {
		return usage.Summary{}, runtimeContractViolation("usage summary returned nil")
	}
	summary := usage.Summary{
		SinceDays: sinceDays, Total: projectUsageTotals(result.Total),
		ByProvider: projectUsageBuckets(result.ByProvider),
		ByModel:    projectUsageBuckets(result.ByModel),
		ByDay:      projectUsageBuckets(result.ByDay),
		Sessions:   result.Sessions, Runs: result.Runs,
	}
	if err := summary.Validate(); err != nil {
		return usage.Summary{}, runtimeContractViolation("usage summary returned an invalid report: %v", err)
	}
	return summary, nil
}

func projectUsageBuckets(values []protocol.UsageBucket) []usage.Bucket {
	projected := make([]usage.Bucket, len(values))
	for index, value := range values {
		projected[index] = usage.Bucket{Key: value.Key, Totals: projectUsageTotals(value.ModelUsage), Runs: value.Runs}
	}
	return projected
}

func projectUsageTotals(value protocol.ModelUsage) usage.Totals {
	projected := usage.Totals{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens,
	}
	if value.CostUSD != nil {
		projected.CostUSD = new(*value.CostUSD)
	}
	return projected
}
