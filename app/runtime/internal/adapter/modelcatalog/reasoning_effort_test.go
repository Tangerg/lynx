package modelcatalog

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

func TestCapabilitiesAdmitSelectionUsesExactCatalogReasoningVocabulary(t *testing.T) {
	supported, err := modelref.NewWithReasoningEffort("openai", "gpt-5.6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Capabilities{}).AdmitSelection(supported); err != nil {
		t.Fatalf("supported effort error = %v", err)
	}

	for _, selection := range []modelref.Selection{
		mustReasoningSelection(t, "openai", "gpt-5.6-sol", "turbo"),
		mustReasoningSelection(t, "alibaba", "qwen-mt-plus", "high"),
	} {
		if err := (Capabilities{}).AdmitSelection(selection); !errors.Is(err, ErrUnsupportedReasoningEffort) {
			t.Fatalf("selection %q/%q/%q error = %v", selection.Provider(), selection.Model(), selection.ReasoningEffort(), err)
		}
	}
}

func TestCapabilitiesAdmitSelectionAllowsPrivateCatalogMiss(t *testing.T) {
	selection := mustReasoningSelection(t, "openai-compatible", "private-reasoning-model", "custom")
	if err := (Capabilities{}).AdmitSelection(selection); err != nil {
		t.Fatalf("private selection error = %v", err)
	}
}

func mustReasoningSelection(t *testing.T, provider, model, effort string) modelref.Selection {
	t.Helper()
	selection, err := modelref.NewWithReasoningEffort(provider, model, effort)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}
