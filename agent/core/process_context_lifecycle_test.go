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
		"terminate process": func() error { return process.Terminate("stop") },
		"cancel tool call":  process.CancelToolCall,
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
