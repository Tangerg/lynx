package modelcatalog

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

func TestLookupTokenLimitsPreservesIndependentCatalogFacts(t *testing.T) {
	selection, err := modelref.New("openai", "gpt-5-pro")
	if err != nil {
		t.Fatal(err)
	}
	limits, found, err := LookupTokenLimits(selection)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("LookupTokenLimits() missed openai/gpt-5-pro")
	}
	if limits.ContextWindow() != 400_000 ||
		limits.MaxInputTokens() != 272_000 ||
		limits.MaxOutputTokens() != 272_000 {
		t.Fatalf(
			"limits = context:%d input:%d output:%d",
			limits.ContextWindow(),
			limits.MaxInputTokens(),
			limits.MaxOutputTokens(),
		)
	}
}

func TestLookupTokenLimitsAllowsPrivateCatalogMiss(t *testing.T) {
	selection, err := modelref.New("openai-compatible", "private-model")
	if err != nil {
		t.Fatal(err)
	}
	limits, found, err := LookupTokenLimits(selection)
	if err != nil || found || !limits.IsZero() {
		t.Fatalf("LookupTokenLimits() = (%+v,%t,%v), want zero,false,nil", limits, found, err)
	}
}
