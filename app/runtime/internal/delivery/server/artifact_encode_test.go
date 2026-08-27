package server

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/sessions"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
)

func TestArtifactFromPortableRejectsUnknownRunEnums(t *testing.T) {
	tests := []struct {
		name     string
		portable sessions.PortableSnapshot
	}{
		{
			name: "outcome",
			portable: sessions.PortableSnapshot{Runs: []sessions.PortableRun{{
				ID: "run_1", Outcome: run.Outcome("invalid"),
			}}},
		},
		{
			name: "run failure",
			portable: sessions.PortableSnapshot{Runs: []sessions.PortableRun{{
				ID: "run_1", Outcome: run.OutcomeFailed,
				Failure: &run.Failure{Kind: run.FailureKind("invalid")},
			}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := artifactFromPortable(test.portable); err == nil {
				t.Fatal("artifact encoding accepted an unknown Run enum")
			}
		})
	}
}
