package runs

import (
	"errors"
	"testing"
)

func TestExecutorRefValidateFor(t *testing.T) {
	valid := ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}
	if err := valid.ValidateFor("ses_1"); err != nil {
		t.Fatalf("valid executor reference: %v", err)
	}
	for name, ref := range map[string]ExecutorRef{
		"missing session":  {ExecutorID: "turn_1"},
		"missing executor": {SessionID: "ses_1"},
		"foreign session":  {SessionID: "ses_2", ExecutorID: "turn_1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ref.ValidateFor("ses_1"); !errors.Is(err, ErrInvalidExecutorRef) {
				t.Fatalf("ValidateFor error = %v, want ErrInvalidExecutorRef", err)
			}
		})
	}
}
