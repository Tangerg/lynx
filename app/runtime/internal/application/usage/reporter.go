// Package usage reports durable Run metering through application-owned read
// models.
package usage

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// RunReader reads the durable run history for one session.
type RunReader interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
}

// SessionLister lists the user-facing sessions that contribute to aggregate
// usage. Child sessions are excluded by the session use case, preventing
// subtree-aggregated runs from being counted twice.
type SessionLister interface {
	List(ctx context.Context) ([]session.Session, error)
}

// Bucket is one named portion of a summary report.
type Bucket struct {
	Key   string
	Usage accounting.Totals
	Runs  int
}

// SessionReport is one session's cumulative metering and per-model split.
type SessionReport struct {
	Total   accounting.Totals
	ByModel map[string]accounting.Totals
}

// Summary is a cross-session usage report. Provider and day buckets reconcile
// with Total because every completed run contributes as one whole run.
type Summary struct {
	Total      accounting.Totals
	ByProvider []Bucket
	ByModel    []Bucket
	ByDay      []Bucket
	Sessions   int
	Runs       int
}

// Dependencies are the durable projections and model policy a Reporter needs.
type Dependencies struct {
	Runs     RunReader
	Sessions SessionLister
	Now      func() time.Time
}

// Reporter folds durable terminal run records into read-only usage reports.
type Reporter struct {
	runs     RunReader
	sessions SessionLister
	now      func() time.Time
}

// New constructs a usage Reporter over the supplied projections.
func New(deps Dependencies) *Reporter {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Reporter{
		runs: deps.Runs, sessions: deps.Sessions, now: now,
	}
}

// Session returns one session's cumulative metering and per-model split.
func (r *Reporter) Session(ctx context.Context, sessionID string) (SessionReport, error) {
	runs, err := r.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return SessionReport{}, err
	}
	total := usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	for _, run := range runs {
		foldRun(run, time.Time{}, &total, nil, byModel, nil)
	}
	report := SessionReport{Total: total.usage()}
	if len(byModel) > 0 {
		report.ByModel = make(map[string]accounting.Totals, len(byModel))
		for name, bucket := range byModel {
			report.ByModel[name] = bucket.usage()
		}
	}
	return report, nil
}

// Summary returns usage across user-facing sessions. A positive sinceDays
// includes runs finished in the preceding calendar duration; zero means all
// durable history.
func (r *Reporter) Summary(ctx context.Context, sinceDays int) (Summary, error) {
	sessions, err := r.sessions.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	var since time.Time
	if sinceDays > 0 {
		since = r.now().UTC().AddDate(0, 0, -sinceDays)
	}

	total := usageAccumulator{}
	byProvider := map[string]*usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	byDay := map[string]*usageAccumulator{}
	sessionCount := 0
	for _, sess := range sessions {
		runs, err := r.runs.ListRuns(ctx, sess.ID)
		if err != nil {
			return Summary{}, err
		}
		before := total.runs
		for _, run := range runs {
			foldRun(run, since, &total, byProvider, byModel, byDay)
		}
		if total.runs > before {
			sessionCount++
		}
	}

	return Summary{
		Total:      total.usage(),
		ByProvider: bucketsBySpend(byProvider),
		ByModel:    bucketsBySpend(byModel),
		ByDay:      bucketsByKey(byDay),
		Sessions:   sessionCount,
		Runs:       total.runs,
	}, nil
}

func foldRun(current run.Run, since time.Time, total *usageAccumulator, byProvider, byModel, byDay map[string]*usageAccumulator) {
	usage, reported := current.Metrics().Usage()
	if !current.State().IsTerminal() || !reported {
		return
	}
	if !since.IsZero() && !current.FinishedAt().IsZero() && current.FinishedAt().Before(since) {
		return
	}
	if total != nil {
		total.add(usage.Total)
		total.runs++
	}
	if byProvider != nil {
		bucket := accumulatorFor(byProvider, current.ModelSelection().Provider())
		bucket.add(usage.Total)
		bucket.runs++
	}
	if byDay != nil && !current.FinishedAt().IsZero() {
		bucket := accumulatorFor(byDay, current.FinishedAt().UTC().Format(time.DateOnly))
		bucket.add(usage.Total)
		bucket.runs++
	}
	if byModel == nil {
		return
	}
	if len(usage.ByModel) > 0 {
		for name, split := range usage.ByModel {
			bucket := accumulatorFor(byModel, name)
			bucket.add(split)
			bucket.runs++
		}
		return
	}
	bucket := accumulatorFor(byModel, current.ModelSelection().Model())
	bucket.add(usage.Total)
	bucket.runs++
}

// usageAccumulator preserves the metering fields needed while folding Run
// records into one report bucket.
type usageAccumulator struct {
	tokens  accounting.Totals
	cost    float64
	hasCost bool
	runs    int
}

func (a *usageAccumulator) add(usage accounting.Totals) {
	a.tokens.InputTokens += usage.InputTokens
	a.tokens.OutputTokens += usage.OutputTokens
	a.tokens.CacheReadTokens += usage.CacheReadTokens
	a.tokens.CacheWriteTokens += usage.CacheWriteTokens
	a.tokens.ReasoningTokens += usage.ReasoningTokens
	if usage.CostUSD != nil {
		a.cost += *usage.CostUSD
		a.hasCost = true
	}
}

func (a usageAccumulator) usage() accounting.Totals {
	out := a.tokens
	if a.hasCost {
		cost := a.cost
		out.CostUSD = &cost
	}
	return out
}

func accumulatorFor(byKey map[string]*usageAccumulator, key string) *usageAccumulator {
	bucket := byKey[key]
	if bucket == nil {
		bucket = &usageAccumulator{}
		byKey[key] = bucket
	}
	return bucket
}

func bucketsBySpend(byKey map[string]*usageAccumulator) []Bucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b Bucket) int {
		return cmp.Or(
			cmp.Compare(costOf(b.Usage.CostUSD), costOf(a.Usage.CostUSD)),
			cmp.Compare(b.Usage.InputTokens, a.Usage.InputTokens),
		)
	})
	return buckets
}

func bucketsByKey(byKey map[string]*usageAccumulator) []Bucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b Bucket) int { return cmp.Compare(a.Key, b.Key) })
	return buckets
}

func bucketsOf(byKey map[string]*usageAccumulator) []Bucket {
	buckets := make([]Bucket, 0, len(byKey))
	for key, accumulated := range byKey {
		buckets = append(buckets, Bucket{Key: key, Usage: accumulated.usage(), Runs: accumulated.runs})
	}
	return buckets
}

func costOf(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}
