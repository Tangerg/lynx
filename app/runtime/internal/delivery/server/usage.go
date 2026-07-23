package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// SessionUsage projects the application-owned session usage report onto the
// usage.session wire contract.
func (s *Server) SessionUsage(ctx context.Context, sessionID string) (*protocol.Usage, error) {
	report, err := s.usage.Session(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return sessionUsageWire(report), nil
}

// UsageSummary projects the application-owned aggregate usage report onto the
// usage.summary wire contract.
func (s *Server) UsageSummary(ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	report, err := s.usage.Summary(ctx, in.SinceDays)
	if err != nil {
		return nil, err
	}
	return &protocol.UsageSummary{
		Total:      usageWire(report.Total),
		ByProvider: usageBucketsWire(report.ByProvider),
		ByModel:    usageBucketsWire(report.ByModel),
		ByDay:      usageBucketsWire(report.ByDay),
		Sessions:   report.Sessions,
		Runs:       report.Runs,
	}, nil
}

func sessionUsageWire(report usage.SessionReport) *protocol.Usage {
	out := &protocol.Usage{ModelUsage: usageWire(report.Total)}
	if len(report.ByModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(report.ByModel))
		for model, modelUsage := range report.ByModel {
			out.ByModel[model] = usageWire(modelUsage)
		}
	}
	return out
}

func usageBucketsWire(buckets []usage.Bucket) []protocol.UsageBucket {
	out := make([]protocol.UsageBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, protocol.UsageBucket{Key: bucket.Key, ModelUsage: usageWire(bucket.Usage), Runs: bucket.Runs})
	}
	return out
}

func usageWire(usage transcript.ModelUsage) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}
