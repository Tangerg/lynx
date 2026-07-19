package config

import (
	"reflect"
	"testing"
)

// TestParseA2AAgents covers the LYRA_A2A_AGENTS env-var parser: the
// name=cardURL shape, URL query strings (split on the first '='), trimming,
// empty entries, and error cases.
func TestParseA2AAgents(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []A2AAgentConfig
		wantErr bool
	}{
		{name: "empty", in: "", want: nil},
		{
			name: "single",
			in:   "weather=https://weather.example.com",
			want: []A2AAgentConfig{{Name: "weather", CardURL: "https://weather.example.com"}},
		},
		{
			name: "multiple with spaces",
			in:   " weather=https://w.example.com , planner=http://localhost:9001 ",
			want: []A2AAgentConfig{
				{Name: "weather", CardURL: "https://w.example.com"},
				{Name: "planner", CardURL: "http://localhost:9001"},
			},
		},
		{
			name: "url with query string splits on first =",
			in:   "search=https://s.example.com/agent?key=abc",
			want: []A2AAgentConfig{{Name: "search", CardURL: "https://s.example.com/agent?key=abc"}},
		},
		{name: "missing =", in: "https://no-name.example.com", wantErr: true},
		{name: "empty name", in: "=https://x.example.com", wantErr: true},
		{name: "empty url", in: "x=", wantErr: true},
		{name: "duplicate name", in: "x=https://one.example,x=https://two.example", wantErr: true},
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

func TestAddA2ARPCOrigins(t *testing.T) {
	agents := []A2AAgentConfig{
		{Name: "weather", CardURL: "https://cards.example"},
		{Name: "planner", CardURL: "https://planner.example"},
	}
	got, err := addA2ARPCOrigins(agents, "weather=https://rpc.example|http://localhost:9001")
	if err != nil {
		t.Fatalf("addA2ARPCOrigins: %v", err)
	}
	want := []string{"https://rpc.example", "http://localhost:9001"}
	if !reflect.DeepEqual(got[0].AllowedRPCOrigins, want) {
		t.Fatalf("weather origins = %v, want %v", got[0].AllowedRPCOrigins, want)
	}
	if got[1].AllowedRPCOrigins != nil {
		t.Fatalf("planner origins = %v, want nil", got[1].AllowedRPCOrigins)
	}
}

func TestAddA2ARPCOriginsRejectsInvalidMappings(t *testing.T) {
	agents := []A2AAgentConfig{{Name: "weather", CardURL: "https://cards.example"}}
	for _, raw := range []string{
		"unknown=https://rpc.example",
		"weather=",
		"weather=https://one.example|",
		"weather=https://one.example,weather=https://two.example",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := addA2ARPCOrigins(agents, raw); err == nil {
				t.Fatalf("addA2ARPCOrigins(%q) succeeded, want error", raw)
			}
			if agents[0].AllowedRPCOrigins != nil {
				t.Fatalf("failed parse mutated input origins: %v", agents[0].AllowedRPCOrigins)
			}
		})
	}
}
