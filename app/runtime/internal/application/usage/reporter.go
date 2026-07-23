// Package usage reports durable run metering without exposing storage or wire
// shapes to its callers.
package usage

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// RunReader reads the durable run history for one session.
type RunReader interface {
	ListRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
}

// SessionLister lists the user-facing sessions that contribute to aggregate
// usage. Child sessions are excluded by the session use case, preventing
// subtree-aggregated runs from being counted twice.
type SessionLister interface {
	List(ctx context.Context) ([]session.Session, error)
}

// ModelDefaults attributes runs that intentionally carry no explicit
// provider/model to the runtime defaults that executed them.
type ModelDefaults interface {
	DefaultProvider() string
	DefaultModel() string
}

// ModelUsage is one token and optional-cost roll-up. A nil CostUSD means none
// of the contributing runs was priced; it is not a synthetic zero.
type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

// Bucket is one named portion of a summary report.
type Bucket struct {
	Key   string
	Usage ModelUsage
	Runs  int
}

// SessionReport is one session's cumulative metering and per-model split.
type SessionReport struct {
	Total   ModelUsage
	ByModel map[string]ModelUsage
}

// Summary is a cross-session usage report. Provider and day buckets reconcile
// with Total because every completed run contributes as one whole run.
type Summary struct {
	Total      ModelUsage
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
	Defaults ModelDefaults
	Now      func() time.Time
}

// Reporter folds durable terminal run records into read-only usage reports.
type Reporter struct {
	runs     RunReader
	sessions SessionLister
	defaults ModelDefaults
	now      func() time.Time
}

// New constructs a usage Reporter over the supplied projections.
func New(deps Dependencies) *Reporter {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Reporter{runs: deps.Runs, sessions: deps.Sessions, defaults: deps.Defaults, now: now}
}

// Session returns one session's cumulative metering and per-model split.
func (r *Reporter) Session(ctx context.Context, sessionID string) (SessionReport, error) {
	runs, err := r.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return SessionReport{}, err
	}
	total := accumulator{}
	byModel := map[string]*accumulator{}
	for _, run := range runs {
		foldRun(run, time.Time{}, r.defaults.DefaultProvider(), r.defaults.DefaultModel(), &total, nil, byModel, nil)
	}
	report := SessionReport{Total: total.usage()}
	if len(byModel) > 0 {
		report.ByModel = make(map[string]ModelUsage, len(byModel))
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

	total := accumulator{}
	byProvider := map[string]*accumulator{}
	byModel := map[string]*accumulator{}
	byDay := map[string]*accumulator{}
	sessionCount := 0
	for _, sess := range sessions {
		runs, err := r.runs.ListRuns(ctx, sess.ID)
		if err != nil {
			return Summary{}, err
		}
		before := total.runs
		for _, run := range runs {
			foldRun(run, since, r.defaults.DefaultProvider(), r.defaults.DefaultModel(), &total, byProvider, byModel, byDay)
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

func foldRun(run transcript.Run, since time.Time, defaultProvider, defaultModel string, total *accumulator, byProvider, byModel, byDay map[string]*accumulator) {
	if !run.State.IsTerminal() || run.Result == nil || run.Result.Usage == nil {
		return
	}
	if !since.IsZero() && !run.FinishedAt.IsZero() && run.FinishedAt.Before(since) {
		return
	}
	usage := modelUsage(run.Result.Usage.ModelUsage)
	if total != nil {
		total.add(usage)
		total.runs++
	}
	if byProvider != nil {
		bucket := accumulatorFor(byProvider, cmp.Or(run.Provider, defaultProvider, "unknown"))
		bucket.add(usage)
		bucket.runs++
	}
	if byDay != nil && !run.FinishedAt.IsZero() {
		bucket := accumulatorFor(byDay, run.FinishedAt.UTC().Format(time.DateOnly))
		bucket.add(usage)
		bucket.runs++
	}
	if byModel == nil {
		return
	}
	if len(run.Result.Usage.ByModel) > 0 {
		for name, split := range run.Result.Usage.ByModel {
			bucket := accumulatorFor(byModel, name)
			bucket.add(modelUsage(split))
			bucket.runs++
		}
		return
	}
	bucket := accumulatorFor(byModel, cmp.Or(run.Model, defaultModel, "unknown"))
	bucket.add(usage)
	bucket.runs++
}

func modelUsage(usage transcript.ModelUsage) ModelUsage {
	return ModelUsage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		ReasoningTokens: usage.ReasoningTokens, CostUSD: usage.CostUSD,
	}
}

type accumulator struct {
	tokens  ModelUsage
	cost    float64
	hasCost bool
	runs    int
}

func (a *accumulator) add(usage ModelUsage) {
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

func (a accumulator) usage() ModelUsage {
	out := a.tokens
	if a.hasCost {
		cost := a.cost
		out.CostUSD = &cost
	}
	return out
}

func accumulatorFor(byKey map[string]*accumulator, key string) *accumulator {
	bucket := byKey[key]
	if bucket == nil {
		bucket = &accumulator{}
		byKey[key] = bucket
	}
	return bucket
}

func bucketsBySpend(byKey map[string]*accumulator) []Bucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b Bucket) int {
		return cmp.Or(
			cmp.Compare(costOf(b.Usage.CostUSD), costOf(a.Usage.CostUSD)),
			cmp.Compare(b.Usage.InputTokens, a.Usage.InputTokens),
		)
	})
	return buckets
}

func bucketsByKey(byKey map[string]*accumulator) []Bucket {
	buckets := bucketsOf(byKey)
	slices.SortFunc(buckets, func(a, b Bucket) int { return cmp.Compare(a.Key, b.Key) })
	return buckets
}

func bucketsOf(byKey map[string]*accumulator) []Bucket {
	buckets := make([]Bucket, 0, len(byKey))
	for key, accumulator := range byKey {
		buckets = append(buckets, Bucket{Key: key, Usage: accumulator.usage(), Runs: accumulator.runs})
	}
	return buckets
}

func costOf(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}
