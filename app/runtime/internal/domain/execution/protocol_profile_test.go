package execution

import "testing"

func TestRunProtocolProfileHasOneDurableRepresentation(t *testing.T) {
	canonical := RunProtocolProfile{
		ChildRuns:      true,
		InterruptKinds: []InterruptKind{ApprovalInterrupt, QuestionInterrupt},
	}
	if err := canonical.Validate(); err != nil {
		t.Fatalf("Validate canonical profile: %v", err)
	}
	for name, profile := range map[string]RunProtocolProfile{
		"duplicate": {
			InterruptKinds: []InterruptKind{ApprovalInterrupt, ApprovalInterrupt},
		},
		"unsorted": {
			InterruptKinds: []InterruptKind{QuestionInterrupt, ApprovalInterrupt},
		},
		"unknown": {
			InterruptKinds: []InterruptKind{InterruptKind(255)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := profile.Validate(); err == nil {
				t.Fatal("Validate accepted a non-canonical protocol profile")
			}
		})
	}

	spelledDifferently := RunProtocolProfile{
		ChildRuns: true,
		InterruptKinds: []InterruptKind{
			QuestionInterrupt,
			ApprovalInterrupt,
			QuestionInterrupt,
		},
	}
	if !canonical.Equal(spelledDifferently) {
		t.Fatal("Equal treated two spellings of the same protocol set as different")
	}
	if canonical.Equal(RunProtocolProfile{InterruptKinds: canonical.InterruptKinds}) {
		t.Fatal("Equal ignored the child-Run contract")
	}
}
