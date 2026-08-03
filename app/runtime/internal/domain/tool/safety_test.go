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
		{name: "delegate_task", want: SafetyClassSafe},
		{name: "list_schedules", want: SafetyClassSafe},
		{name: "create_schedule", want: SafetyClassWrite},
		{name: "write", want: SafetyClassWrite},
		{name: "shell", want: SafetyClassExec},
		{name: "read_shell_output", want: SafetyClassSafe},
		{name: "stop_shell", want: SafetyClassExec},
		{name: "search_memory", want: SafetyClassSafe},
		{name: "search_conversations", want: SafetyClassSafe},
		{name: "search_tools", want: SafetyClassSafe},
		{name: "web_fetch", want: SafetyClassNetwork},
		{name: "web_search", want: SafetyClassNetwork},
		{name: "http_request", want: SafetyClassNetwork},
		{name: "unknown_tool", want: SafetyClassExec},
	} {
		if got := SafetyClassFor(test.name); got != test.want {
			t.Errorf("SafetyClassFor(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}
