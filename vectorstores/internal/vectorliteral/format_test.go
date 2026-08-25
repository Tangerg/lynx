package vectorliteral

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name   string
		vector []float32
		want   string
	}{
		{name: "empty", want: "[]"},
		{name: "values", vector: []float32{1, -2.5, 0.25}, want: "[1,-2.5,0.25]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Format(test.vector); got != test.want {
				t.Fatalf("Format(%v) = %q, want %q", test.vector, got, test.want)
			}
		})
	}
}
