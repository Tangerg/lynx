package secretmask

import "testing"

func TestMask(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", want: ""},
		{name: "short", value: "abc123", want: "****"},
		{name: "boundary", value: "12345678", want: "****"},
		{name: "long", value: "sk-ant-0000fc78", want: "sk****78"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Mask(test.value); got != test.want {
				t.Fatalf("Mask(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
