package provider

import "testing"

func TestProviderEnabled(t *testing.T) {
	if (Provider{}).Enabled() {
		t.Error("empty provider should not be enabled")
	}
	if !(Provider{APIKey: "k"}).Enabled() {
		t.Error("keyed provider should be enabled")
	}
}

func TestPatchDistinguishesPreserveReplaceAndClear(t *testing.T) {
	provider := Provider{ID: "openai", APIKey: "sk-old", BaseURL: "https://old.test"}

	baseURL := "https://new.test"
	updated := provider.Apply(Patch{BaseURL: &baseURL})
	if updated.APIKey != provider.APIKey || updated.BaseURL != baseURL {
		t.Fatalf("replace endpoint = %+v", updated)
	}

	emptyAPIKey := ""
	updated = updated.Apply(Patch{APIKey: &emptyAPIKey})
	if updated.APIKey != "" || updated.BaseURL != baseURL {
		t.Fatalf("clear key = %+v", updated)
	}
	if !(Patch{}).Empty() || (Patch{APIKey: &emptyAPIKey}).Empty() {
		t.Fatal("Patch.Empty does not distinguish preserve from clear")
	}
}
