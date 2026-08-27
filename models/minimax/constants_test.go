package minimax

import "testing"

func TestRegionalBaseURLs(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"international OpenAI":    {BaseURLIntl, "https://api.minimax.io/v1"},
		"China OpenAI":            {BaseURLChina, "https://api.minimaxi.com/v1"},
		"international Anthropic": {BaseURLIntlAnthropic, "https://api.minimax.io/anthropic"},
		"China Anthropic":         {BaseURLChinaAnthropic, "https://api.minimaxi.com/anthropic"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("base URL = %q, want %q", test.got, test.want)
			}
		})
	}
}
