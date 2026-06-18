package config

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/a2a"
)

// TestParseA2AAgents covers the LYRA_A2A_AGENTS env-var parser: the
// name=cardURL shape, URL query strings (split on the first '='), trimming,
// empty entries, and error cases.
func TestParseA2AAgents(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []a2a.ClientConfig
		wantErr bool
	}{
		{name: "empty", in: "", want: nil},
		{
			name: "single",
			in:   "weather=https://weather.example.com",
			want: []a2a.ClientConfig{{Name: "weather", CardURL: "https://weather.example.com"}},
		},
		{
			name: "multiple with spaces",
			in:   " weather=https://w.example.com , planner=http://localhost:9001 ",
			want: []a2a.ClientConfig{
				{Name: "weather", CardURL: "https://w.example.com"},
				{Name: "planner", CardURL: "http://localhost:9001"},
			},
		},
		{
			name: "url with query string splits on first =",
			in:   "search=https://s.example.com/agent?key=abc",
			want: []a2a.ClientConfig{{Name: "search", CardURL: "https://s.example.com/agent?key=abc"}},
		},
		{name: "missing =", in: "https://no-name.example.com", wantErr: true},
		{name: "empty name", in: "=https://x.example.com", wantErr: true},
		{name: "empty url", in: "x=", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseA2AAgents(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseA2AAgents(%q) = nil error, want error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseA2AAgents(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseA2AAgents(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
