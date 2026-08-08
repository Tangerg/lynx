package run

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
)

func TestRunCapabilitiesHasOneDurableRepresentation(t *testing.T) {
	canonical := RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	if err := canonical.Validate(); err != nil {
		t.Fatalf("Validate canonical capabilities: %v", err)
	}
	for name, capabilities := range map[string]RunCapabilities{
		"duplicate": {
			InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Approval},
		},
		"unsorted": {
			InterruptKinds: []interrupt.Kind{interrupt.Question, interrupt.Approval},
		},
		"unknown": {
			InterruptKinds: []interrupt.Kind{interrupt.Kind(255)},
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
		InterruptKinds: []interrupt.Kind{
			interrupt.Question,
			interrupt.Approval,
			interrupt.Question,
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
	original := RunCapabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}}
	cloned := original.Clone()
	cloned.InterruptKinds[0] = interrupt.Question
	if original.InterruptKinds[0] != interrupt.Approval {
		t.Fatal("Clone shares interrupt-kind storage with the source capabilities")
	}
}

func TestRunCapabilitiesReportsTheCompleteMissingSet(t *testing.T) {
	required := RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	caller := RunCapabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}}
	missing := required.MissingFrom(caller)
	if !missing.ChildRuns || len(missing.InterruptKinds) != 1 || missing.InterruptKinds[0] != interrupt.Question {
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
