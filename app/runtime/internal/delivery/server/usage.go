package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/usage"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// SessionUsage projects the application-owned session usage report onto the
// usage.session wire contract.
func (s *Server) SessionUsage(ctx context.Context, sessionID string) (*protocol.Usage, error) {
	report, err := s.usage.Session(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return presentSessionUsage(report), nil
}

// UsageSummary projects the application-owned aggregate usage report onto the
// usage.summary wire contract.
func (s *Server) UsageSummary(ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	report, err := s.usage.Summary(ctx, in.SinceDays)
	if err != nil {
		return nil, err
	}
	return &protocol.UsageSummary{
		Total:      presentModelUsage(report.Total),
		ByProvider: presentUsageBuckets(report.ByProvider),
		ByModel:    presentUsageBuckets(report.ByModel),
		ByDay:      presentUsageBuckets(report.ByDay),
		Sessions:   report.Sessions,
		Runs:       report.Runs,
	}, nil
}

func presentSessionUsage(report usage.SessionReport) *protocol.Usage {
	out := &protocol.Usage{ModelUsage: presentModelUsage(report.Total)}
	if len(report.ByModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(report.ByModel))
		for model, modelUsage := range report.ByModel {
			out.ByModel[model] = presentModelUsage(modelUsage)
		}
	}
	return out
}

func presentUsageBuckets(buckets []usage.Bucket) []protocol.UsageBucket {
	out := make([]protocol.UsageBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, protocol.UsageBucket{Key: bucket.Key, ModelUsage: presentModelUsage(bucket.Usage), Runs: bucket.Runs})
	}
	return out
}
