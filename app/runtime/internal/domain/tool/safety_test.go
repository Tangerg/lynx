package tool

import "testing"

func TestSafetyClassValueSemantics(t *testing.T) {
	for _, test := range []struct {
		class SafetyClass
		risk  RiskLevel
	}{
		{class: SafetyClassSafe, risk: RiskLow},
		{class: SafetyClassWrite, risk: RiskMedium},
		{class: SafetyClassExec, risk: RiskHigh},
		{class: SafetyClassNetwork, risk: RiskHigh},
	} {
		if !test.class.Valid() || test.class.Risk() != test.risk {
			t.Errorf("class %q: valid=%v risk=%q; want risk %q", test.class, test.class.Valid(), test.class.Risk(), test.risk)
		}
		if !test.risk.Valid() {
			t.Errorf("risk %q is invalid", test.risk)
		}
	}

	var zero SafetyClass
	if zero.Valid() || zero.Risk() != RiskHigh {
		t.Fatalf("zero class: valid=%v risk=%q, want invalid/high", zero.Valid(), zero.Risk())
	}
	var zeroRisk RiskLevel
	if zeroRisk.Valid() {
		t.Fatal("zero risk is valid")
	}
}

func TestSafetyClassForUsesConservativeDefaults(t *testing.T) {
	for _, test := range []struct {
		name string
		want SafetyClass
	}{
		{name: "task", want: SafetyClassSafe},
		{name: "write", want: SafetyClassWrite},
		{name: "shell", want: SafetyClassExec},
		{name: "unknown_tool", want: SafetyClassExec},
	} {
		if got := SafetyClassFor(test.name); got != test.want {
			t.Errorf("SafetyClassFor(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}
