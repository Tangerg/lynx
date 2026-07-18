package runs

import (
	"errors"
	"testing"
)

func TestTurnRefValidateFor(t *testing.T) {
	valid := TurnRef{SessionID: "ses_1", TurnID: "turn_1"}
	if err := valid.ValidateFor("ses_1"); err != nil {
		t.Fatalf("valid turn reference: %v", err)
	}
	for name, ref := range map[string]TurnRef{
		"missing session": {TurnID: "turn_1"},
		"missing turn":    {SessionID: "ses_1"},
		"foreign session": {SessionID: "ses_2", TurnID: "turn_1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ref.ValidateFor("ses_1"); !errors.Is(err, ErrInvalidTurnRef) {
				t.Fatalf("ValidateFor error = %v, want ErrInvalidTurnRef", err)
			}
		})
	}
}
