package core_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func TestProcessContextLifecycleErrorsIdentifyUnavailableControl(t *testing.T) {
	process := core.NewProcessContext(core.ProcessContextConfig{})
	for name, run := range map[string]func() error{
		"terminate agent":     func() error { return process.TerminateAgent("stop") },
		"terminate action":    func() error { return process.TerminateAction("stop") },
		"terminate tool call": process.TerminateToolCall,
		"suspend": func() error {
			_, err := process.Suspend(t.Context(), interaction.Suspension{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, core.ErrLifecycleControlUnavailable) {
				t.Fatalf("error = %v, want ErrLifecycleControlUnavailable", err)
			}
		})
	}
}

func TestParallelProcessContextLifecycleErrorsRemainSpecific(t *testing.T) {
	process := core.NewProcessContext(core.ProcessContextConfig{})
	branch, err := process.ForParallelBranch()
	if err != nil {
		t.Fatal(err)
	}
	if err := branch.TerminateAgent("stop"); !errors.Is(err, core.ErrParallelBranchControl) {
		t.Fatalf("error = %v, want ErrParallelBranchControl", err)
	}
}
