package turn

import "testing"

func TestSetRestoredProcessRejectsInvalidPhaseWithoutPublishing(t *testing.T) {
	state := newRestoringTurnState(t.Context(), TurnHandle{})
	state.phase = turnTerminal

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("setRestoredProcess did not reject an invalid phase")
			}
		}()
		state.setRestoredProcess(nil)
	}()
	if state.process() != nil {
		t.Fatal("invalid restored-process publication mutated state")
	}
}
