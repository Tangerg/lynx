package terminal

import (
	"testing"
	"time"
)

func TestPressConfirmationRequiresTheSameGestureInsideTheWindow(t *testing.T) {
	var confirmation pressConfirmation
	now := time.Unix(100, 0)
	if confirmation.Confirm(confirmQuit, now) {
		t.Fatal("the first press confirmed quit")
	}
	if confirmation.Confirm(confirmClearDraft, now.Add(time.Millisecond)) {
		t.Fatal("a different gesture confirmed the armed action")
	}
	if !confirmation.Confirm(confirmClearDraft, now.Add(2*time.Millisecond)) {
		t.Fatal("the matching second press did not confirm")
	}
	if confirmation.Armed(confirmClearDraft) {
		t.Fatal("a confirmed gesture remained armed")
	}
}

func TestPressConfirmationExpires(t *testing.T) {
	var confirmation pressConfirmation
	now := time.Unix(100, 0)
	confirmation.Confirm(confirmQuit, now)
	if confirmation.Confirm(confirmQuit, now.Add(confirmationWindow+time.Nanosecond)) {
		t.Fatal("an expired gesture confirmed")
	}
	if !confirmation.Confirm(confirmQuit, now.Add(confirmationWindow+time.Millisecond)) {
		t.Fatal("the press after rearming did not confirm")
	}
}
