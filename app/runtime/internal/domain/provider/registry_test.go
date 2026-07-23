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
