package operationsflow

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/accounting"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (service *Service) SessionUsage(
	ctx context.Context,
	sessionID string,
) (*protocol.Usage, error) {
	exists, err := service.store.SessionExists(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, protocol.ErrSessionNotFound
	}
	records, err := service.store.ListUsageRunRecords(ctx, sessionID, time.Time{})
	if err != nil {
		return nil, err
	}

	total := usageAccumulator{}
	byModel := make(map[string]*usageAccumulator)
	for _, record := range records {
		if record.Usage == nil {
			continue
		}
		total.Add(record.Usage.ModelUsage)
		if len(record.Usage.ByModel) == 0 {
			modelBucket(byModel, record.Provider, record.Model).Add(record.Usage.ModelUsage)
			continue
		}
		for model, usage := range record.Usage.ByModel {
			modelBucket(byModel, record.Provider, model).Add(usage)
		}
	}

	result := &protocol.Usage{ModelUsage: presentUsage(total.Value())}
	if len(byModel) > 0 {
		result.ByModel = make(map[string]protocol.ModelUsage, len(byModel))
		for key, value := range byModel {
			result.ByModel[key] = presentUsage(value.Value())
		}
	}
	return result, nil
}

func (service *Service) UsageSummary(
	ctx context.Context,
	request protocol.UsageSummaryRequest,
) (*protocol.UsageSummary, error) {
	if request.SinceDays < 0 {
		return nil, fmt.Errorf("%w: sinceDays must be non-negative", protocol.ErrInvalidParams)
	}
	var since time.Time
	if request.SinceDays > 0 {
		since = service.now().UTC().AddDate(0, 0, -request.SinceDays)
	}
	records, err := service.store.ListUsageRunRecords(ctx, "", since)
	if err != nil {
		return nil, err
	}

	total := usageAccumulator{}
	providers := make(map[string]*bucketAccumulator)
	models := make(map[string]*bucketAccumulator)
	days := make(map[string]*bucketAccumulator)
	sessions := make(map[string]bool)
	runs := 0
	for _, record := range records {
		if record.Usage == nil {
			continue
		}
		usage := record.Usage.ModelUsage
		total.Add(usage)
		bucket(providers, record.Provider).Add(usage)
		if len(record.Usage.ByModel) == 0 {
			bucket(models, modelKey(record.Provider, record.Model)).Add(usage)
		} else {
			for model, modelUsage := range record.Usage.ByModel {
				bucket(models, modelKey(record.Provider, model)).Add(modelUsage)
			}
		}
		bucket(days, record.FinishedAt.UTC().Format(time.DateOnly)).Add(usage)
		sessions[record.SessionID] = true
		runs++
	}
	return &protocol.UsageSummary{
		Total:      presentUsage(total.Value()),
		ByProvider: presentBuckets(providers, false),
		ByModel:    presentBuckets(models, false),
		ByDay:      presentBuckets(days, true),
		Sessions:   len(sessions),
		Runs:       runs,
	}, nil
}

type usageAccumulator struct {
	usage       accounting.ModelUsage
	contributed bool
	costKnown   bool
}

func (accumulator *usageAccumulator) Add(value accounting.ModelUsage) {
	accumulator.usage.InputTokens += value.InputTokens
	accumulator.usage.OutputTokens += value.OutputTokens
	accumulator.usage.CacheReadTokens += value.CacheReadTokens
	accumulator.usage.CacheWriteTokens += value.CacheWriteTokens
	accumulator.usage.ReasoningTokens += value.ReasoningTokens
	if !accumulator.contributed {
		accumulator.costKnown = value.CostUSD != nil
	} else if value.CostUSD == nil {
		accumulator.costKnown = false
	}
	if value.CostUSD != nil {
		if accumulator.usage.CostUSD == nil {
			cost := 0.0
			accumulator.usage.CostUSD = &cost
		}
		*accumulator.usage.CostUSD += *value.CostUSD
	}
	accumulator.contributed = true
}

func (accumulator usageAccumulator) Value() accounting.ModelUsage {
	value := accumulator.usage
	if !accumulator.contributed || !accumulator.costKnown {
		value.CostUSD = nil
	}
	return value
}

type bucketAccumulator struct {
	usageAccumulator
	runs int
}

func (accumulator *bucketAccumulator) Add(value accounting.ModelUsage) {
	accumulator.usageAccumulator.Add(value)
	accumulator.runs++
}

func modelBucket(
	values map[string]*usageAccumulator,
	provider string,
	model string,
) *usageAccumulator {
	key := modelKey(provider, model)
	value := values[key]
	if value == nil {
		value = &usageAccumulator{}
		values[key] = value
	}
	return value
}

func modelKey(provider, model string) string {
	return provider + "/" + model
}

func bucket(
	values map[string]*bucketAccumulator,
	key string,
) *bucketAccumulator {
	value := values[key]
	if value == nil {
		value = &bucketAccumulator{}
		values[key] = value
	}
	return value
}

func presentBuckets(
	values map[string]*bucketAccumulator,
	keyOrder bool,
) []protocol.UsageBucket {
	result := make([]protocol.UsageBucket, 0, len(values))
	for key, value := range values {
		result = append(result, protocol.UsageBucket{
			Key:        key,
			ModelUsage: presentUsage(value.Value()),
			Runs:       value.runs,
		})
	}
	slices.SortFunc(result, func(left, right protocol.UsageBucket) int {
		if keyOrder {
			return strings.Compare(left.Key, right.Key)
		}
		leftTokens := left.InputTokens + left.OutputTokens
		rightTokens := right.InputTokens + right.OutputTokens
		if leftTokens > rightTokens {
			return -1
		}
		if leftTokens < rightTokens {
			return 1
		}
		return strings.Compare(left.Key, right.Key)
	})
	return result
}

func presentUsage(value accounting.ModelUsage) protocol.ModelUsage {
	return protocol.ModelUsage{
		InputTokens:      value.InputTokens,
		OutputTokens:     value.OutputTokens,
		CacheReadTokens:  value.CacheReadTokens,
		CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens:  value.ReasoningTokens,
		CostUSD:          cloneCost(value.CostUSD),
	}
}

func cloneCost(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
