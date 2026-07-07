package provider

import "testing"

func TestProviderMaskedAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"unconfigured", "", ""},
		{"short fully masked", "abc123", "****"},
		{"boundary 8 fully masked", "12345678", "****"},
		{"long reveals ends", "sk-ant-0000fc78", "sk****78"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Provider{APIKey: tc.key}
			if got := p.MaskedAPIKey(); got != tc.want {
				t.Errorf("MaskedAPIKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderEnabled(t *testing.T) {
	if (Provider{}).Enabled() {
		t.Error("empty provider should not be enabled")
	}
	if !(Provider{APIKey: "k"}).Enabled() {
		t.Error("keyed provider should be enabled")
	}
}
