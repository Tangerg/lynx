package execution

import (
	"errors"
	"strings"
	"testing"
)

func TestRunCapabilitiesHasOneDurableRepresentation(t *testing.T) {
	canonical := RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []InterruptKind{ApprovalInterrupt, QuestionInterrupt},
	}
	if err := canonical.Validate(); err != nil {
		t.Fatalf("Validate canonical capabilities: %v", err)
	}
	for name, capabilities := range map[string]RunCapabilities{
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
			if err := capabilities.Validate(); err == nil {
				t.Fatal("Validate accepted a non-canonical capability set")
			}
		})
	}

	spelledDifferently := RunCapabilities{
		ChildRuns: true,
		InterruptKinds: []InterruptKind{
			QuestionInterrupt,
			ApprovalInterrupt,
			QuestionInterrupt,
		},
	}
	if !canonical.Equal(spelledDifferently) {
		t.Fatal("Equal treated two spellings of the same capability set as different")
	}
	if canonical.Equal(RunCapabilities{InterruptKinds: canonical.InterruptKinds}) {
		t.Fatal("Equal ignored the child-Run capability")
	}
}

func TestRunCapabilitiesCloneOwnsInterruptKinds(t *testing.T) {
	original := RunCapabilities{InterruptKinds: []InterruptKind{ApprovalInterrupt}}
	cloned := original.Clone()
	cloned.InterruptKinds[0] = QuestionInterrupt
	if original.InterruptKinds[0] != ApprovalInterrupt {
		t.Fatal("Clone shares interrupt-kind storage with the source capabilities")
	}
}

func TestRunCapabilitiesReportsTheCompleteMissingSet(t *testing.T) {
	required := RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []InterruptKind{ApprovalInterrupt, QuestionInterrupt},
	}
	caller := RunCapabilities{InterruptKinds: []InterruptKind{ApprovalInterrupt}}
	missing := required.MissingFrom(caller)
	if !missing.ChildRuns || len(missing.InterruptKinds) != 1 || missing.InterruptKinds[0] != QuestionInterrupt {
		t.Fatalf("MissingFrom() = %v, want child runs and question interrupts", missing)
	}
	if text := missing.String(); strings.Contains(text, "interruptTypes") || text != "child runs, question interrupts" {
		t.Fatalf("String() = %q, want semantic capability names", text)
	}

	err := &InsufficientCapabilities{RunID: "run_1", Missing: missing}
	if !errors.Is(err, ErrInsufficientCapabilities) || !strings.Contains(err.Error(), `run "run_1" requires child runs, question interrupts`) {
		t.Fatalf("InsufficientCapabilities = %v", err)
	}
}
