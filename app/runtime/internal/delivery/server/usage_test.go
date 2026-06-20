package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func usd(v float64) *float64 { return &v }

func finishedRunBlob(t *testing.T, provider, model string, at time.Time, u protocol.Usage) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(protocol.RunRef{
		ID:         "run_x",
		Provider:   provider,
		Model:      model,
		Status:     protocol.RunStatusFinished,
		FinishedAt: at,
		Outcome: &protocol.RunOutcome{
			Type:   protocol.OutcomeCompleted,
			Result: &protocol.RunResult{Usage: &u},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFoldRunUsage_FoldsAllDimensions(t *testing.T) {
	day := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	blob := finishedRunBlob(t, "anthropic", "claude-opus-4-8", day, protocol.Usage{
		ModelUsage: protocol.ModelUsage{InputTokens: 100, OutputTokens: 40, CostUSD: usd(1.5)},
	})

	total := usageAcc{}
	byProvider := map[string]*usageAcc{}
	byModel := map[string]*usageAcc{}
	byDay := map[string]*usageAcc{}
	foldRunUsage(blob, time.Time{}, "openai", "gpt", &total, byProvider, byModel, byDay)

	if total.runs != 1 || total.tok.InputTokens != 100 || total.cost != 1.5 {
		t.Fatalf("total = %+v", total)
	}
	if byProvider["anthropic"] == nil || byProvider["anthropic"].tok.OutputTokens != 40 {
		t.Errorf("byProvider missing anthropic: %+v", byProvider)
	}
	if byModel["claude-opus-4-8"] == nil {
		t.Errorf("byModel missing model: %+v", byModel)
	}
	if byDay["2026-06-21"] == nil {
		t.Errorf("byDay missing 2026-06-21: %+v", byDay)
	}
}

func TestFoldRunUsage_DefaultsAttributeUnnamedRuns(t *testing.T) {
	// A default-model run carries no provider/model — attribute to the defaults.
	blob := finishedRunBlob(t, "", "", time.Now().UTC(), protocol.Usage{
		ModelUsage: protocol.ModelUsage{InputTokens: 10},
	})
	byProvider := map[string]*usageAcc{}
	byModel := map[string]*usageAcc{}
	foldRunUsage(blob, time.Time{}, "anthropic", "claude-opus-4-8", nil, byProvider, byModel, nil)

	if byProvider["anthropic"] == nil {
		t.Errorf("default provider not attributed: %+v", byProvider)
	}
	if byModel["claude-opus-4-8"] == nil {
		t.Errorf("default model not attributed: %+v", byModel)
	}
}

func TestFoldRunUsage_PrefersByModelSplit(t *testing.T) {
	// A run that touched two models (headline + utility) splits per model.
	blob := finishedRunBlob(t, "anthropic", "claude-opus-4-8", time.Now().UTC(), protocol.Usage{
		ModelUsage: protocol.ModelUsage{InputTokens: 120, CostUSD: usd(2)},
		ByModel: map[string]protocol.ModelUsage{
			"claude-opus-4-8":  {InputTokens: 100, CostUSD: usd(1.8)},
			"claude-haiku-4-5": {InputTokens: 20, CostUSD: usd(0.2)},
		},
	})
	byModel := map[string]*usageAcc{}
	foldRunUsage(blob, time.Time{}, "", "", nil, nil, byModel, nil)

	if len(byModel) != 2 {
		t.Fatalf("expected 2 model buckets, got %+v", byModel)
	}
	if byModel["claude-haiku-4-5"] == nil || byModel["claude-haiku-4-5"].tok.InputTokens != 20 {
		t.Errorf("utility model not split out: %+v", byModel)
	}
}

func TestFoldRunUsage_SkipsUnfinishedAndOld(t *testing.T) {
	total := usageAcc{}

	// Not finished → skipped.
	running, _ := json.Marshal(protocol.RunRef{Status: protocol.RunStatusRunning})
	foldRunUsage(running, time.Time{}, "", "", &total, nil, nil, nil)

	// Finished but no usage → skipped.
	noUsage, _ := json.Marshal(protocol.RunRef{
		Status:  protocol.RunStatusFinished,
		Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}},
	})
	foldRunUsage(noUsage, time.Time{}, "", "", &total, nil, nil, nil)

	// Finished before the since cutoff → skipped.
	old := finishedRunBlob(t, "anthropic", "m", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: 99}})
	foldRunUsage(old, time.Now().UTC().AddDate(0, 0, -1), "", "", &total, nil, nil, nil)

	if total.runs != 0 {
		t.Errorf("expected nothing folded, got runs=%d tokens=%d", total.runs, total.tok.InputTokens)
	}
}

func TestUsageAcc_CostOmittedWhenUnpriced(t *testing.T) {
	a := usageAcc{}
	a.add(protocol.ModelUsage{InputTokens: 10}) // no cost
	if got := a.modelUsage(); got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil (omit when unpriced)", *got.CostUSD)
	}
	a.add(protocol.ModelUsage{InputTokens: 5, CostUSD: usd(0.3)})
	if got := a.modelUsage(); got.CostUSD == nil || *got.CostUSD != 0.3 {
		t.Errorf("CostUSD = %v, want 0.3", got.CostUSD)
	}
}

func TestBucketsBySpend_RanksByCostDesc(t *testing.T) {
	m := map[string]*usageAcc{
		"cheap": {tok: protocol.ModelUsage{InputTokens: 1}, cost: 0.1, hasCost: true},
		"dear":  {tok: protocol.ModelUsage{InputTokens: 1}, cost: 9, hasCost: true},
	}
	out := bucketsBySpend(m)
	if out[0].Key != "dear" {
		t.Errorf("expected dear first (spend-ranked), got %+v", out)
	}
}
