package usage

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func usd(v float64) *float64 { return &v }

func finishedRun(t *testing.T, provider, model string, at time.Time, usage transcript.Usage) transcript.Run {
	t.Helper()
	return transcript.Run{
		ID: "run_x", Provider: provider, Model: model, State: execution.Completed,
		FinishedAt: at, Result: &transcript.RunResult{Usage: &usage},
	}
}

func TestFoldRunFoldsAllDimensions(t *testing.T) {
	day := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	run := finishedRun(t, "anthropic", "claude-opus-4-8", day, transcript.Usage{
		ModelUsage: transcript.ModelUsage{InputTokens: 100, OutputTokens: 40, CostUSD: usd(1.5)},
	})

	total := accumulator{}
	byProvider := map[string]*accumulator{}
	byModel := map[string]*accumulator{}
	byDay := map[string]*accumulator{}
	foldRun(run, time.Time{}, "openai", "gpt", &total, byProvider, byModel, byDay)

	if total.runs != 1 || total.tokens.InputTokens != 100 || total.cost != 1.5 {
		t.Fatalf("total = %+v", total)
	}
	if byProvider["anthropic"] == nil || byProvider["anthropic"].tokens.OutputTokens != 40 {
		t.Errorf("byProvider missing anthropic: %+v", byProvider)
	}
	if byModel["claude-opus-4-8"] == nil {
		t.Errorf("byModel missing model: %+v", byModel)
	}
	if byDay["2026-06-21"] == nil {
		t.Errorf("byDay missing 2026-06-21: %+v", byDay)
	}
}

func TestFoldRunDefaultsAttributeUnnamedRuns(t *testing.T) {
	run := finishedRun(t, "", "", time.Now().UTC(), transcript.Usage{
		ModelUsage: transcript.ModelUsage{InputTokens: 10},
	})
	byProvider := map[string]*accumulator{}
	byModel := map[string]*accumulator{}
	foldRun(run, time.Time{}, "anthropic", "claude-opus-4-8", nil, byProvider, byModel, nil)

	if byProvider["anthropic"] == nil {
		t.Errorf("default provider not attributed: %+v", byProvider)
	}
	if byModel["claude-opus-4-8"] == nil {
		t.Errorf("default model not attributed: %+v", byModel)
	}
}

type staticRunReader map[string][]transcript.Run

func (r staticRunReader) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	return r[sessionID], nil
}

type staticSessionLister []session.Session

func (l staticSessionLister) List(context.Context) ([]session.Session, error) { return l, nil }

func TestReporterUsesConfiguredDefaults(t *testing.T) {
	reporter := New(Dependencies{
		Runs: staticRunReader{
			"ses_1": {finishedRun(t, "", "", time.Now().UTC(), transcript.Usage{
				ModelUsage: transcript.ModelUsage{InputTokens: 10},
			})},
		},
		Sessions:        staticSessionLister{{ID: "ses_1"}},
		DefaultProvider: "anthropic",
		DefaultModel:    "claude-opus-4-8",
	})

	summary, err := reporter.Summary(t.Context(), 0)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.ByProvider) != 1 || summary.ByProvider[0].Key != "anthropic" {
		t.Fatalf("provider buckets = %+v, want anthropic", summary.ByProvider)
	}
	if len(summary.ByModel) != 1 || summary.ByModel[0].Key != "claude-opus-4-8" {
		t.Fatalf("model buckets = %+v, want configured default", summary.ByModel)
	}
}

func TestFoldRunPrefersByModelSplit(t *testing.T) {
	run := finishedRun(t, "anthropic", "claude-opus-4-8", time.Now().UTC(), transcript.Usage{
		ModelUsage: transcript.ModelUsage{InputTokens: 120, CostUSD: usd(2)},
		ByModel: map[string]transcript.ModelUsage{
			"claude-opus-4-8":  {InputTokens: 100, CostUSD: usd(1.8)},
			"claude-haiku-4-5": {InputTokens: 20, CostUSD: usd(0.2)},
		},
	})
	byModel := map[string]*accumulator{}
	foldRun(run, time.Time{}, "", "", nil, nil, byModel, nil)

	if len(byModel) != 2 {
		t.Fatalf("expected 2 model buckets, got %+v", byModel)
	}
	if byModel["claude-haiku-4-5"] == nil || byModel["claude-haiku-4-5"].tokens.InputTokens != 20 {
		t.Errorf("utility model not split out: %+v", byModel)
	}
}

func TestFoldRunSkipsUnfinishedAndOld(t *testing.T) {
	total := accumulator{}

	foldRun(transcript.Run{State: execution.Running}, time.Time{}, "", "", &total, nil, nil, nil)
	noUsage := transcript.Run{State: execution.Completed, Result: &transcript.RunResult{}}
	foldRun(noUsage, time.Time{}, "", "", &total, nil, nil, nil)
	old := finishedRun(t, "anthropic", "m", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		transcript.Usage{ModelUsage: transcript.ModelUsage{InputTokens: 99}})
	foldRun(old, time.Now().UTC().AddDate(0, 0, -1), "", "", &total, nil, nil, nil)

	if total.runs != 0 {
		t.Errorf("expected nothing folded, got runs=%d tokens=%d", total.runs, total.tokens.InputTokens)
	}
}

func TestAccumulatorOmitsCostWhenUnpriced(t *testing.T) {
	a := accumulator{}
	a.add(transcript.ModelUsage{InputTokens: 10})
	if got := a.usage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil", *got.CostUSD)
	}
	a.add(transcript.ModelUsage{InputTokens: 5, CostUSD: usd(0.3)})
	if got := a.usage(); got.CostUSD == nil || *got.CostUSD != 0.3 {
		t.Errorf("CostUSD = %v, want 0.3", got.CostUSD)
	}
}

func TestBucketsBySpendRanksByCostDesc(t *testing.T) {
	m := map[string]*accumulator{
		"cheap": {tokens: transcript.ModelUsage{InputTokens: 1}, cost: 0.1, hasCost: true},
		"dear":  {tokens: transcript.ModelUsage{InputTokens: 1}, cost: 9, hasCost: true},
	}
	out := bucketsBySpend(m)
	if out[0].Key != "dear" {
		t.Errorf("expected dear first (spend-ranked), got %+v", out)
	}
}
