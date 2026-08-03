package plan

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		steps   []Step
		wantErr bool
	}{
		{name: "empty clears plan"},
		{name: "all pending", steps: []Step{{Description: "inspect", Status: StatusPending}, {Description: "implement", Status: StatusPending}}},
		{name: "one in progress", steps: []Step{{Description: "inspect", Status: StatusCompleted}, {Description: "implement", Status: StatusInProgress}}},
		{name: "multiple completed", steps: []Step{{Description: "inspect", Status: StatusCompleted}, {Description: "implement", Status: StatusCompleted}}},
		{name: "missing description", steps: []Step{{Status: StatusPending}}, wantErr: true},
		{name: "blank description", steps: []Step{{Description: "  ", Status: StatusPending}}, wantErr: true},
		{name: "unknown status", steps: []Step{{Description: "inspect", Status: "done"}}, wantErr: true},
		{name: "two in progress", steps: []Step{{Description: "inspect", Status: StatusInProgress}, {Description: "implement", Status: StatusInProgress}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.steps)
			if test.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate error = %v, want ErrInvalid", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate error = %v", err)
			}
		})
	}
}
