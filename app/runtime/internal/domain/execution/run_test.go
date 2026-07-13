package execution

import "testing"

var allStates = []RunState{Running, Interrupted, Completed, Failed, Canceled}

var allOutcomes = []Outcome{OutcomeCompleted, OutcomeCanceled, OutcomeError, OutcomeMaxBudget, OutcomeMaxSteps}

// TestIsTerminal pins the terminal set: exactly Completed / Failed / Canceled.
func TestIsTerminal(t *testing.T) {
	terminal := map[RunState]bool{Completed: true, Failed: true, Canceled: true}
	for _, s := range allStates {
		if got := s.IsTerminal(); got != terminal[s] {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, terminal[s])
		}
	}
}

// TestSuspend: only Running parks (→ Interrupted).
func TestSuspend(t *testing.T) {
	for _, s := range allStates {
		got, ok := s.Suspend()
		wantOK := s == Running
		wantState := s
		if wantOK {
			wantState = Interrupted
		}
		if got != wantState || ok != wantOK {
			t.Errorf("%s.Suspend() = (%s,%v), want (%s,%v)", s, got, ok, wantState, wantOK)
		}
	}
}

// TestResume: only Interrupted continues (→ Running).
func TestResume(t *testing.T) {
	for _, s := range allStates {
		got, ok := s.Resume()
		wantOK := s == Interrupted
		wantState := s
		if wantOK {
			wantState = Running
		}
		if got != wantState || ok != wantOK {
			t.Errorf("%s.Resume() = (%s,%v), want (%s,%v)", s, got, ok, wantState, wantOK)
		}
	}
}

// TestTerminate is the full (state × outcome) matrix. Running terminates for any
// outcome; Interrupted terminates only via cancel; all other states reject.
func TestTerminate(t *testing.T) {
	for _, s := range allStates {
		for _, o := range allOutcomes {
			got, ok := s.Terminate(o)
			var wantState RunState
			var wantOK bool
			switch {
			case s == Running:
				wantState, wantOK = o.terminalState(), true
			case s == Interrupted && o == OutcomeCanceled:
				wantState, wantOK = Canceled, true
			default:
				wantState, wantOK = s, false
			}
			if got != wantState || ok != wantOK {
				t.Errorf("%s.Terminate(%s) = (%s,%v), want (%s,%v)", s, o, got, ok, wantState, wantOK)
			}
			if ok && !got.IsTerminal() {
				t.Errorf("%s.Terminate(%s) produced non-terminal %s", s, o, got)
			}
		}
	}
}

// TestOutcomeTerminalState pins the outcome → terminal-state mapping: completion
// and cancellation get their own states; every failure flavor folds into Failed.
func TestOutcomeTerminalState(t *testing.T) {
	want := map[Outcome]RunState{
		OutcomeCompleted: Completed,
		OutcomeCanceled:  Canceled,
		OutcomeError:     Failed,
		OutcomeMaxBudget: Failed,
		OutcomeMaxSteps:  Failed,
	}
	for _, o := range allOutcomes {
		if got := o.terminalState(); got != want[o] {
			t.Errorf("%s.terminalState() = %s, want %s", o, got, want[o])
		}
	}
}

// TestNoTransitionFromTerminal: once terminal, no operation advances the run.
func TestNoTransitionFromTerminal(t *testing.T) {
	for _, s := range []RunState{Completed, Failed, Canceled} {
		if _, ok := s.Suspend(); ok {
			t.Errorf("%s.Suspend() unexpectedly succeeded", s)
		}
		if _, ok := s.Resume(); ok {
			t.Errorf("%s.Resume() unexpectedly succeeded", s)
		}
		for _, o := range allOutcomes {
			if _, ok := s.Terminate(o); ok {
				t.Errorf("%s.Terminate(%s) unexpectedly succeeded", s, o)
			}
		}
	}
}

// TestStringsAreDistinct guards the String() maps against a copy-paste collision
// (two states or two outcomes sharing a label) and the "unknown" fallthrough.
func TestStringsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range allStates {
		if s.String() == "unknown" {
			t.Errorf("state %d stringifies as unknown", s)
		}
		if seen[s.String()] {
			t.Errorf("duplicate state label %q", s.String())
		}
		seen[s.String()] = true
	}
	seen = map[string]bool{}
	for _, o := range allOutcomes {
		if o.String() == "unknown" {
			t.Errorf("outcome %d stringifies as unknown", o)
		}
		if seen[o.String()] {
			t.Errorf("duplicate outcome label %q", o.String())
		}
		seen[o.String()] = true
	}
}

// TestDurability pins the commit-before-publish predicate.
