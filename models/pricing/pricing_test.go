package pricing

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
		if ok && got.InputPer1M != c.wantInput {
			t.Errorf("Lookup(%q,%q) input = %v, want %v", c.provider, c.model, got.InputPer1M, c.wantInput)
		}
	}
}

// TestCatalogIntegrity guards the embedded configs: every cataloged
// model must have a positive input + output rate (a zero usually means a
// typo'd / missing field), so cost attribution is never silently free.
func TestCatalogIntegrity(t *testing.T) {
	if len(catalog) == 0 {
		t.Fatal("catalog is empty — embedded configs failed to load")
	}
	for provider, models := range catalog {
		if len(models) == 0 {
			t.Errorf("provider %q has no models", provider)
		}
		for id, p := range models {
			if p.InputPer1M <= 0 || p.OutputPer1M <= 0 {
				t.Errorf("%s/%s: input=%v output=%v, both must be > 0", provider, id, p.InputPer1M, p.OutputPer1M)
			}
		}
	}
}
