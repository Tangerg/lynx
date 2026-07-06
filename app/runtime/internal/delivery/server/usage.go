package server

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// Usage reporting (usage.session / usage.summary). Sum-on-read over the durable
// run history (history_runs): each finished run already carries its terminal
// metering (RunResult.Usage, subtree-aggregated), so totals are a fold over run
// blobs — no denormalized counters to keep in sync, and a rollback/fork that
// drops runs is reflected for free (the dropped runs are simply gone).
//
// Attribution is at run granularity: per-provider / per-day buckets fold whole
// runs, so they reconcile with the grand total; per-model uses each run's own
// ByModel split when present (which captures the utility / sub-agent models a
// run touched) and otherwise the run's headline model.

// SessionUsage returns one session's cumulative token usage + cost, summed over
// its finished runs (usage.session).
func (s *Server) SessionUsage(ctx context.Context, sessionID string) (*protocol.Usage, error) {
	runs, err := s.transcript.ListTranscriptRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	total := usageAcc{}
	byModel := map[string]*usageAcc{}
	for _, r := range runs {
		foldRunUsage(r.Blob, time.Time{}, s.providers.DefaultProvider(), s.sessions.DefaultModel(), &total, nil, byModel, nil)
	}
	out := &protocol.Usage{ModelUsage: total.modelUsage()}
	if len(byModel) > 0 {
		out.ByModel = make(map[string]protocol.ModelUsage, len(byModel))
		for k, v := range byModel {
			out.ByModel[k] = v.modelUsage()
		}
	}
	return out, nil
}

// UsageSummary returns a cross-session spend report (usage.summary). It iterates
// the user-facing sessions (session.List excludes sub-agent children, so a
// parent's subtree-aggregated runs aren't double-counted against the children's
// own run records) and folds each one's finished runs.
func (s *Server) UsageSummary(ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	sessions, err := s.sessions.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var since time.Time
	if in.SinceDays > 0 {
		since = time.Now().UTC().AddDate(0, 0, -in.SinceDays)
	}

	total := usageAcc{}
	byProvider := map[string]*usageAcc{}
	byModel := map[string]*usageAcc{}
	byDay := map[string]*usageAcc{}
	sessionCount := 0
	for _, sess := range sessions {
		runs, lerr := s.transcript.ListTranscriptRuns(ctx, sess.ID)
		if lerr != nil {
			return nil, lerr
		}
		before := total.runs
		for _, r := range runs {
			foldRunUsage(r.Blob, since, s.providers.DefaultProvider(), s.sessions.DefaultModel(), &total, byProvider, byModel, byDay)
		}
		if total.runs > before {
			sessionCount++
		}
	}

	return &protocol.UsageSummary{
		Total:      total.modelUsage(),
		ByProvider: bucketsBySpend(byProvider),
		ByModel:    bucketsBySpend(byModel),
		ByDay:      bucketsByKey(byDay),
		Sessions:   sessionCount,
		Runs:       total.runs,
	}, nil
}

// foldRunUsage lifts one run blob's terminal usage and folds it into the
// supplied accumulators. The grouped maps (byProvider/byModel/byDay) are
// optional — pass nil to skip (usage.session wants only total + byModel). since
// (when non-zero) drops runs finished before it. defProvider / defModel attribute
// default-model runs (whose RunRef carries no provider/model). Pure (no Server
// receiver) so the aggregation is unit-testable from crafted blobs.
func foldRunUsage(blob json.RawMessage, since time.Time, defProvider, defModel string, total *usageAcc, byProvider, byModel, byDay map[string]*usageAcc) {
	var ref protocol.RunRef
	if json.Unmarshal(blob, &ref) != nil {
		return
	}
	if ref.Status != protocol.RunStatusFinished || ref.Outcome == nil ||
		ref.Outcome.Result == nil || ref.Outcome.Result.Usage == nil {
		return
	}
	if !since.IsZero() && !ref.FinishedAt.IsZero() && ref.FinishedAt.Before(since) {
		return
	}
	u := ref.Outcome.Result.Usage

	if total != nil {
		total.add(u.ModelUsage)
		total.runs++
	}
	if byProvider != nil {
		b := accFor(byProvider, firstNonEmpty(ref.Provider, defProvider, "unknown"))
		b.add(u.ModelUsage)
		b.runs++
	}
	if byDay != nil && !ref.FinishedAt.IsZero() {
		b := accFor(byDay, ref.FinishedAt.UTC().Format(time.DateOnly))
		b.add(u.ModelUsage)
		b.runs++
	}
	if byModel != nil {
		// Prefer the run's own per-model split (captures utility / sub-agent
		// models); fall back to the headline model when the run reports no split.
		if len(u.ByModel) > 0 {
			for name, mu := range u.ByModel {
				b := accFor(byModel, name)
				b.add(mu)
				b.runs++
			}
		} else {
			b := accFor(byModel, firstNonEmpty(ref.Model, defModel, "unknown"))
			b.add(u.ModelUsage)
			b.runs++
		}
	}
}

// usageAcc accumulates token counts plus an optional cost (cost stays absent
// until a priced run contributes, mirroring ModelUsage.CostUSD's omit-when-
// unpriced contract — never faked to 0) and the run tally.
type usageAcc struct {
	tok     protocol.ModelUsage
	cost    float64
	hasCost bool
	runs    int
}

func (a *usageAcc) add(m protocol.ModelUsage) {
	a.tok.InputTokens += m.InputTokens
	a.tok.OutputTokens += m.OutputTokens
	a.tok.CacheReadTokens += m.CacheReadTokens
	a.tok.CacheWriteTokens += m.CacheWriteTokens
	a.tok.ReasoningTokens += m.ReasoningTokens
	if m.CostUSD != nil {
		a.cost += *m.CostUSD
		a.hasCost = true
	}
}

func (a usageAcc) modelUsage() protocol.ModelUsage {
	out := a.tok
	if a.hasCost {
		c := a.cost
		out.CostUSD = &c
	}
	return out
}

func accFor(m map[string]*usageAcc, key string) *usageAcc {
	a := m[key]
	if a == nil {
		a = &usageAcc{}
		m[key] = a
	}
	return a
}

// bucketsBySpend renders a key→acc map as wire buckets, sorted by cost desc then
// input tokens desc — the spend-ranked order a dashboard wants.
func bucketsBySpend(m map[string]*usageAcc) []protocol.UsageBucket {
	out := bucketsOf(m)
	sort.Slice(out, func(i, j int) bool {
		ci, cj := costOf(out[i].CostUSD), costOf(out[j].CostUSD)
		if ci != cj {
			return ci > cj
		}
		return out[i].InputTokens > out[j].InputTokens
	})
	return out
}

// bucketsByKey renders buckets sorted by key ascending (chronological for days).
func bucketsByKey(m map[string]*usageAcc) []protocol.UsageBucket {
	out := bucketsOf(m)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func bucketsOf(m map[string]*usageAcc) []protocol.UsageBucket {
	out := make([]protocol.UsageBucket, 0, len(m))
	for k, v := range m {
		out = append(out, protocol.UsageBucket{Key: k, ModelUsage: v.modelUsage(), Runs: v.runs})
	}
	return out
}

func costOf(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
