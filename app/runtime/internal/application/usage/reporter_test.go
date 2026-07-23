package usage

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
	a.add(ModelUsage{InputTokens: 10})
	if got := a.usage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil", *got.CostUSD)
	}
	a.add(ModelUsage{InputTokens: 5, CostUSD: usd(0.3)})
	if got := a.usage(); got.CostUSD == nil || *got.CostUSD != 0.3 {
		t.Errorf("CostUSD = %v, want 0.3", got.CostUSD)
	}
}

func TestBucketsBySpendRanksByCostDesc(t *testing.T) {
	m := map[string]*accumulator{
		"cheap": {tokens: ModelUsage{InputTokens: 1}, cost: 0.1, hasCost: true},
		"dear":  {tokens: ModelUsage{InputTokens: 1}, cost: 9, hasCost: true},
	}
	out := bucketsBySpend(m)
	if out[0].Key != "dear" {
		t.Errorf("expected dear first (spend-ranked), got %+v", out)
	}
}
