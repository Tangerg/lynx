package usage

import "testing"

func TestUsageReportsRejectNegativeAndDuplicateValues(t *testing.T) {
	cost := 1.25
	report := SessionReport{
		SessionID: "ses_1", Total: Totals{InputTokens: 10, CostUSD: &cost},
		ByModel: []Bucket{{Key: "provider/model", Runs: 1}},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.ByModel = append(report.ByModel, report.ByModel[0])
	if err := report.Validate(); err == nil {
		t.Fatal("duplicate model bucket was accepted")
	}
	summary := Summary{Total: Totals{InputTokens: -1}}
	if err := summary.Validate(); err == nil {
		t.Fatal("negative usage was accepted")
	}
}
