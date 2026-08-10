package usage

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func usd(v float64) *float64 { return &v }

func finishedRun(t *testing.T, provider, model string, at time.Time, usage accounting.Usage) run.Run {
	t.Helper()
	return runfixture.MustRestore(run.Snapshot{ID: "run_x", ModelSelection: mustUsageSelection(t, provider, model), State: run.Completed,
		FinishedAt: at, Metrics: runfixture.MustMetrics(runfixture.MetricsInput{Usage: &usage})})

}

func mustUsageSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func TestFoldRunFoldsAllDimensions(t *testing.T) {
	day := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	run := finishedRun(t, "anthropic", "claude-opus-4-8", day, accounting.Usage{
		Total: accounting.Totals{InputTokens: 100, OutputTokens: 40, CostUSD: usd(1.5)},
	})

	total := usageAccumulator{}
	byProvider := map[string]*usageAccumulator{}
	byModel := map[string]*usageAccumulator{}
	byDay := map[string]*usageAccumulator{}
	foldRun(run, time.Time{}, &total, byProvider, byModel, byDay)

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

func TestFoldRunPrefersByModelSplit(t *testing.T) {
	run := finishedRun(t, "anthropic", "claude-opus-4-8", time.Now().UTC(), accounting.Usage{
		Total: accounting.Totals{InputTokens: 120, CostUSD: usd(2)},
		ByModel: map[string]accounting.Totals{
			"claude-opus-4-8":  {InputTokens: 100, CostUSD: usd(1.8)},
			"claude-haiku-4-5": {InputTokens: 20, CostUSD: usd(0.2)},
		},
	})
	byModel := map[string]*usageAccumulator{}
	foldRun(run, time.Time{}, nil, nil, byModel, nil)

	if len(byModel) != 2 {
		t.Fatalf("expected 2 model buckets, got %+v", byModel)
	}
	if byModel["claude-haiku-4-5"] == nil || byModel["claude-haiku-4-5"].tokens.InputTokens != 20 {
		t.Errorf("utility model not split out: %+v", byModel)
	}
}

func TestFoldRunSkipsUnfinishedAndOld(t *testing.T) {
	total := usageAccumulator{}

	foldRun(runfixture.MustRestore(run.Snapshot{State: run.Running}), time.Time{}, &total, nil, nil, nil)
	noUsage := runfixture.MustRestore(run.Snapshot{State: run.Completed})
	foldRun(noUsage, time.Time{}, &total, nil, nil, nil)
	old := finishedRun(t, "anthropic", "m", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		accounting.Usage{Total: accounting.Totals{InputTokens: 99}})
	foldRun(old, time.Now().UTC().AddDate(0, 0, -1), &total, nil, nil, nil)

	if total.runs != 0 {
		t.Errorf("expected nothing folded, got runs=%d tokens=%d", total.runs, total.tokens.InputTokens)
	}
}

func TestAccumulatorOmitsCostWhenUnpriced(t *testing.T) {
	a := usageAccumulator{}
	a.add(accounting.Totals{InputTokens: 10})
	if got := a.usage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil", *got.CostUSD)
	}
	a.add(accounting.Totals{InputTokens: 5, CostUSD: usd(0.3)})
	if got := a.usage(); got.CostUSD == nil || *got.CostUSD != 0.3 {
		t.Errorf("CostUSD = %v, want 0.3", got.CostUSD)
	}
}

func TestBucketsBySpendRanksByCostDesc(t *testing.T) {
	m := map[string]*usageAccumulator{
		"cheap": {tokens: accounting.Totals{InputTokens: 1}, cost: 0.1, hasCost: true},
		"dear":  {tokens: accounting.Totals{InputTokens: 1}, cost: 9, hasCost: true},
	}
	out := bucketsBySpend(m)
	if out[0].Key != "dear" {
		t.Errorf("expected dear first (spend-ranked), got %+v", out)
	}
}
