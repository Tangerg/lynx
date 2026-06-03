package catalog

import "testing"

func TestLookup(t *testing.T) {
	cases := []struct {
		provider, model string
		wantOK          bool
		wantInput       float64
	}{
		{"anthropic", "claude-sonnet-4-6", true, 3},
		{"Anthropic", "claude-opus-4-8", true, 5},             // case-insensitive provider
		{"anthropic", "claude-3-5-haiku-20241022", true, 0.8}, // lyra's default
		{"openai", "gpt-4o-mini", true, 0.15},                 // lyra's default
		{"OpenAI", "gpt-5", true, 1.25},
		{"anthropic", "no-such-model", false, 0},
		{"made-up", "gpt-4o", false, 0},
	}
	for _, c := range cases {
		got, ok := Lookup(c.provider, c.model)
		if ok != c.wantOK {
			t.Errorf("Lookup(%q,%q) ok = %v, want %v", c.provider, c.model, ok, c.wantOK)
			continue
		}
		if ok && (len(got.Pricing) == 0 || got.Pricing[0].InputPer1M != c.wantInput) {
			t.Errorf("Lookup(%q,%q) base input = %v, want %v", c.provider, c.model, got.Pricing, c.wantInput)
		}
	}
}

// TestModels returns every cataloged model for a provider.
func TestModels(t *testing.T) {
	got := Models("anthropic")
	if len(got) == 0 {
		t.Fatal("Models(anthropic) returned nothing")
	}
	if Models("made-up") != nil {
		t.Error("Models(made-up) should be nil for an uncataloged provider")
	}
}

// TestCatalogIntegrity guards the embedded configs: every cataloged model
// must have an id, and every pricing band must have a positive input rate.
// Output may be zero — some endpoints bill input only (e.g. Azure's
// model-router). Metadata-only rows (no pricing) are allowed — a zero
// input rate is the real red flag (a half-set / typo'd band). Tiered
// bands must also be ascending by threshold, which CostOf relies on.
func TestCatalogIntegrity(t *testing.T) {
	if len(catalog) == 0 {
		t.Fatal("catalog is empty — embedded configs failed to load")
	}
	for provider, models := range catalog {
		if len(models) == 0 {
			t.Errorf("provider %q has no models", provider)
		}
		for id, m := range models {
			if id == "" {
				t.Errorf("provider %q has a model with empty id", provider)
			}
			var prev int64 = -1
			for _, band := range m.Pricing {
				if band.InputPer1M <= 0 {
					t.Errorf("%s/%s: band threshold=%d input=%v, a priced band needs a positive input rate",
						provider, id, band.Threshold, band.InputPer1M)
				}
				if band.Threshold <= prev {
					t.Errorf("%s/%s: pricing bands not ascending by threshold (%d after %d)",
						provider, id, band.Threshold, prev)
				}
				prev = band.Threshold
			}
		}
	}
}
