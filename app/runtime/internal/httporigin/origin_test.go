package httporigin

import "testing"

func TestSame(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{"same origin different path", "https://EXAMPLE.com/a", "https://example.com/b", true},
		{"default HTTPS port", "https://example.com/a", "https://example.com:443/b", true},
		{"different host", "https://a.example/mcp", "https://b.example/mcp", false},
		{"different port", "https://example.com:8443/mcp", "https://example.com/mcp", false},
		{"different scheme", "http://example.com/mcp", "https://example.com/mcp", false},
		{"invalid endpoint", "not a URL", "not a URL", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Same(test.left, test.right); got != test.want {
				t.Fatalf("Same(%q, %q) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}
