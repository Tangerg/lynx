package minimax

import "testing"

func TestRegionalBaseURLs(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"international": {BaseURLIntl, "https://api.minimax.io/v1"},
		"China":         {BaseURLChina, "https://api.minimaxi.com/v1"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("base URL = %q, want %q", test.got, test.want)
			}
		})
	}
}
