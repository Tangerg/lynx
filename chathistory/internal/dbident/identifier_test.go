package dbident

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "chat_history", want: true},
		{value: "_history2", want: true},
		{value: "", want: false},
		{value: "2history", want: false},
		{value: "history-items", want: false},
		{value: `history"; DROP TABLE messages; --`, want: false},
	}

	for _, test := range tests {
		if got := Valid(test.value); got != test.want {
			t.Errorf("Valid(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}
