package accounting

import (
	"math"
	"testing"
)

func TestBudgetValidate(t *testing.T) {
	for _, test := range []struct {
		name    string
		budget  Budget
		wantErr bool
	}{
		{name: "unbounded"},
		{name: "bounded", budget: Budget{MaxTokens: 1, MaxCostUSD: 0.5, MaxSteps: 1}},
		{name: "negative tokens", budget: Budget{MaxTokens: -1}, wantErr: true},
		{name: "negative cost", budget: Budget{MaxCostUSD: -1}, wantErr: true},
		{name: "not a number cost", budget: Budget{MaxCostUSD: math.NaN()}, wantErr: true},
		{name: "positive infinite cost", budget: Budget{MaxCostUSD: math.Inf(1)}, wantErr: true},
		{name: "negative infinite cost", budget: Budget{MaxCostUSD: math.Inf(-1)}, wantErr: true},
		{name: "negative steps", budget: Budget{MaxSteps: -1}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.budget.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
