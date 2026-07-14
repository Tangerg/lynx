package catalog

import "testing"

func TestCatalogIntegrity(t *testing.T) {
	if len(entries) == 0 {
		t.Fatal("catalog is empty")
	}
	for provider, models := range entries {
		if len(models) == 0 {
			t.Errorf("provider %q has no models", provider)
		}
		for id, model := range models {
			if id == "" || model.ID == "" {
				t.Errorf("provider %q has a model with empty id", provider)
			}
			previous := int64(-1)
			for _, band := range model.Pricing {
				if band.InputPer1M <= 0 {
					t.Errorf("%s/%s: priced band threshold=%d has input rate %v", provider, id, band.Threshold, band.InputPer1M)
				}
				if band.Threshold <= previous {
					t.Errorf("%s/%s: pricing bands are not ascending", provider, id)
				}
				previous = band.Threshold
			}
		}
	}
}

func TestLookupReturnsOwnedSlices(t *testing.T) {
	first, ok := Lookup("anthropic", "claude-sonnet-4-6")
	if !ok || len(first.Pricing) == 0 {
		t.Fatal("fixture missing")
	}
	first.Pricing[0].InputPer1M = -1
	second, _ := Lookup("anthropic", "claude-sonnet-4-6")
	if second.Pricing[0].InputPer1M == -1 {
		t.Fatal("Lookup returned catalog-owned pricing slice")
	}
}
